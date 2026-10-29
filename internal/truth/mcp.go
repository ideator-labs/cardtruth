package truth

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ideator-labs/cardtruth/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverCard holds the Server Card members this tool compares at runtime.
type serverCard struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Remotes []struct {
		Type                      string   `json:"type"`
		URL                       string   `json:"url"`
		SupportedProtocolVersions []string `json:"supportedProtocolVersions"`
	} `json:"remotes"`
}

var (
	cardSchemaOnce sync.Once
	cardSchema     *jsonschema.Resolved
	cardSchemaErr  error
)

func resolvedCardSchema() (*jsonschema.Resolved, error) {
	cardSchemaOnce.Do(func() {
		var root jsonschema.Schema
		if cardSchemaErr = json.Unmarshal(schema.ServerCard, &root); cardSchemaErr != nil {
			return
		}
		root.Ref = "#/$defs/ServerCard"
		cardSchema, cardSchemaErr = root.Resolve(nil)
	})
	return cardSchema, cardSchemaErr
}

// ValidateServerCard checks a card against the vendored SEP-2127 schema.
func ValidateServerCard(card []byte) error {
	rs, err := resolvedCardSchema()
	if err != nil {
		return err
	}
	var v any
	if err := json.Unmarshal(card, &v); err != nil {
		return err
	}
	return rs.Validate(v)
}

func (r *run) mcp(ctx context.Context, e rawEntry, raw []byte) Endpoint {
	ep := Endpoint{Protocol: "mcp", Identifier: e.Identifier, CardURL: e.URL}
	sc := Check{Entry: e.Identifier, ID: "mcp.card.schema", Rule: "SEP-2127 schema.json #/$defs/ServerCard"}
	if err := ValidateServerCard(raw); err != nil {
		sc.Status, sc.Got = Fail, err.Error()
	} else {
		sc.Status = Pass
	}
	r.add(sc)

	var card serverCard
	_ = json.Unmarshal(raw, &card)
	ep.Name, ep.Version = card.Name, card.Version
	i := slices.IndexFunc(card.Remotes, func(rm struct {
		Type                      string   `json:"type"`
		URL                       string   `json:"url"`
		SupportedProtocolVersions []string `json:"supportedProtocolVersions"`
	}) bool {
		return rm.Type == "streamable-http" && !strings.HasPrefix(rm.URL, "{")
	})
	if i < 0 {
		r.add(Check{Entry: e.Identifier, ID: "mcp.live.connect", Status: Skip, Got: "no concrete streamable-http remote", Rule: "SEP-2127 remotes[]"})
		return ep
	}
	remote := card.Remotes[i]
	ep.URL = remote.URL

	conn, ok := r.liveCheck(e.Identifier, "mcp.live.connect", "go-sdk Client.Connect (server/discover, initialize fallback)")
	if !ok {
		for _, id := range []string{"mcp.live.name", "mcp.live.version", "mcp.live.protocol"} {
			r.liveCheck(e.Identifier, id, "")
		}
		return ep
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "cardtruth", Version: "0.1.0"}, nil)
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: remote.URL, HTTPClient: r.opt.HTTP}, nil)
	if err != nil {
		conn.Status, conn.Want, conn.Got = Fail, remote.URL, err.Error()
		r.add(conn)
		return ep
	}
	defer sess.Close()
	conn.Status, conn.Got = Pass, remote.URL
	r.add(conn)

	ir := sess.InitializeResult()
	var liveName, liveVersion string
	if ir.ServerInfo != nil {
		liveName, liveVersion = ir.ServerInfo.Name, ir.ServerInfo.Version
	}
	rule := "ext-server-card docs/discovery.md: Consistency with Runtime Behavior"
	r.add(compare(Check{Entry: e.Identifier, ID: "mcp.live.name", Rule: rule}, card.Name, liveName))
	r.add(compare(Check{Entry: e.Identifier, ID: "mcp.live.version", Rule: rule}, card.Version, liveVersion))

	pv := Check{Entry: e.Identifier, ID: "mcp.live.protocol", Rule: rule, Want: strings.Join(remote.SupportedProtocolVersions, ","), Got: ir.ProtocolVersion}
	switch {
	case len(remote.SupportedProtocolVersions) == 0:
		pv.Status = Skip
	case slices.Contains(remote.SupportedProtocolVersions, ir.ProtocolVersion):
		pv.Status = Pass
	default:
		pv.Status = Fail
	}
	r.add(pv)
	return ep
}
