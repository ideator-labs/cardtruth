// Package fixture serves one publisher host: an AI Catalog, an MCP server
// built on the official Go SDK, and an A2A agent built on a2a-go. Each
// scenario introduces one kind of drift between what the catalog and cards
// claim and what the live endpoints do. Tool and agent replies are stubs.
package fixture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Scenarios maps each scenario to the check IDs it must fail.
var Scenarios = map[string][]string{
	"honest":         nil,
	"stale-mcp-card": {"mcp.live.version"},
	"a2a-moved":      {"a2a.live.card", "a2a.live.send"},
	"card-edited":    {"entry.digest"},
	// The card claims only 2026-07-28, but a stateful go-sdk v1.8.0
	// StreamableHTTPHandler cannot serve it and the client falls back to
	// initialize at 2025-11-25.
	"protocol-overclaim": {"mcp.live.protocol"},
}

const (
	mcpName        = "com.acme/finance"
	publishedVer   = "1.2.0"
	upgradedVer    = "1.3.0"
	a2aName        = "Acme Finance Agent"
	cardDescr      = "Quotes and FX rates for Acme (stub data)."
	editedDescr    = "Quotes, FX rates and crypto trading for Acme (stub data)."
	serverCardType = "application/mcp-server-card+json"
	agentCardType  = "application/a2a-agent-card+json"
)

func Handler(base, scenario string) (http.Handler, error) {
	if _, ok := Scenarios[scenario]; !ok {
		return nil, fmt.Errorf("unknown scenario %q", scenario)
	}

	versions := []string{"2026-07-28", "2025-11-25"}
	stateless := true
	if scenario == "protocol-overclaim" {
		versions = []string{"2026-07-28"}
		stateless = false
	}
	published := serverCard(base, cardDescr, versions)
	served := published
	if scenario == "card-edited" {
		served = serverCard(base, editedDescr, versions)
	}

	liveVer := publishedVer
	if scenario == "stale-mcp-card" {
		liveVer = upgradedVer
	}

	liveCard := agentCard(base + "/a2a/invoke")
	catalogCard := liveCard
	if scenario == "a2a-moved" {
		catalogCard = agentCard(base + "/a2a/v1/invoke")
	}
	catalogCardBytes, _ := json.Marshal(catalogCard)

	cat := map[string]any{
		"specVersion": "1.0",
		"host":        map[string]any{"displayName": "Acme (fixture)", "identifier": "did:web:acme.test"},
		"entries": []any{map[string]any{
			"identifier":  "urn:air:acme.test:agent:finance",
			"displayName": "Acme Finance Agent",
			"type":        "application/ai-catalog+json",
			"description": "Finance agent reachable over MCP and A2A.",
			"data": map[string]any{
				"specVersion": "1.0",
				"entries": []any{
					map[string]any{
						"identifier": "urn:air:acme.test:mcp:finance",
						"type":       serverCardType,
						"url":        base + "/mcp/server-card",
						"digest":     digest(published),
					},
					map[string]any{
						"identifier": "urn:air:acme.test:a2a:finance",
						"type":       agentCardType,
						"url":        base + "/agents/finance.json",
						"digest":     digest(catalogCardBytes),
					},
				},
			},
		}},
	}
	catBytes, _ := json.MarshalIndent(cat, "", "  ")

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: mcpName, Version: liveVer}, nil)
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "quote", Description: "Stub quote."},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct {
			Symbol string `json:"symbol"`
		}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "STUB 100.00 " + in.Symbol}}}, nil, nil
		})

	mux := http.NewServeMux()
	mux.Handle("GET /.well-known/ai-catalog.json", static(catBytes, "application/ai-catalog+json"))
	mux.Handle("GET /mcp/server-card", static(served, serverCardType))
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, &mcp.StreamableHTTPOptions{Stateless: stateless}))
	mux.Handle("GET /agents/finance.json", static(catalogCardBytes, agentCardType))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(liveCard))
	mux.Handle("/a2a/invoke", a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(stubAgent{})))
	return mux, nil
}

func serverCard(base, descr string, versions []string) []byte {
	b, _ := json.MarshalIndent(map[string]any{
		"$schema":     "https://static.modelcontextprotocol.io/schemas/v1/server-card.schema.json",
		"name":        mcpName,
		"version":     publishedVer,
		"description": descr,
		"remotes": []any{map[string]any{
			"type":                      "streamable-http",
			"url":                       base + "/mcp",
			"supportedProtocolVersions": versions,
		}},
	}, "", "  ")
	return b
}

func agentCard(invokeURL string) *a2a.AgentCard {
	return &a2a.AgentCard{
		Name:                a2aName,
		Description:         cardDescr,
		Version:             publishedVer,
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(invokeURL, a2a.TransportProtocolJSONRPC)},
		DefaultInputModes:   []string{"text/plain"},
		DefaultOutputModes:  []string{"text/plain"},
		Skills:              []a2a.AgentSkill{{ID: "quote", Name: "Quote", Description: "Stub quote.", Tags: []string{"finance"}}},
	}
}

func static(b []byte, ctype string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", ctype)
		w.Write(b)
	})
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// stubAgent answers every message with a fixed text. No model is called.
type stubAgent struct{}

func (stubAgent) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("STUB reply, no model called")), nil)
	}
}

func (stubAgent) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {}
}
