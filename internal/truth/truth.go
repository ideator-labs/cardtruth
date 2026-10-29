// Package truth walks an AI Catalog, fetches the MCP Server Cards and A2A
// Agent Cards it lists, and checks each card's claims against the live
// endpoint the card points at.
package truth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Agent-Card/ai-catalog-go/catalog"
	"github.com/Agent-Card/ai-catalog-go/trust"
	"github.com/Agent-Card/ai-catalog-go/validate"
)

type Options struct {
	Live bool
	HTTP *http.Client
}

// rawEntry reads the entry members this tool needs straight from the served
// bytes. ai-catalog-go v0.3.0 has no entry-level digest field, while the spec
// at Agent-Card/ai-catalog@9aac2c1 defines one.
type rawEntry struct {
	Identifier  string          `json:"identifier"`
	DisplayName string          `json:"displayName"`
	Type        string          `json:"type"`
	URL         string          `json:"url"`
	Data        json.RawMessage `json:"data"`
	Digest      string          `json:"digest"`
}

type rawCatalog struct {
	Entries []rawEntry `json:"entries"`
}

type run struct {
	opt    Options
	report *Report
}

func Run(ctx context.Context, catalogURL string, opt Options) (*Report, error) {
	if opt.HTTP == nil {
		opt.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	r := &run{opt: opt, report: &Report{Catalog: catalogURL, Live: opt.Live, Agents: []Agent{}, Checks: []Check{}}}

	body, err := r.fetch(ctx, catalogURL, catalog.MediaTypeCatalog)
	if err != nil {
		return nil, err
	}
	cat, err := catalog.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	res := validate.Validate(cat)
	sdk := Check{Entry: catalogURL, ID: "catalog.sdk-valid", Rule: "ai-catalog-go validate.Validate", Want: "no errors", Got: res.ConformanceLevel.String()}
	sdk.Status = Pass
	if !res.IsValid {
		sdk.Status = Fail
		sdk.Got = res.Errors[0].Message
	}
	r.add(sdk)

	var top rawCatalog
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	for _, e := range top.Entries {
		agent := Agent{Identifier: e.Identifier, DisplayName: e.DisplayName, Endpoints: []Endpoint{}}
		leaves := []rawEntry{e}
		if e.Type == catalog.MediaTypeCatalog {
			leaves, err = r.nested(ctx, e)
			if err != nil {
				r.add(Check{Entry: e.Identifier, ID: "catalog.nested", Status: Fail, Got: err.Error(), Rule: "AI Catalog nested catalog entry"})
				continue
			}
		}
		for _, leaf := range leaves {
			if ep, ok := r.leaf(ctx, leaf); ok {
				agent.Endpoints = append(agent.Endpoints, ep)
			}
		}
		r.report.Agents = append(r.report.Agents, agent)
	}
	return r.report, nil
}

func (r *run) add(c Check) { r.report.Checks = append(r.report.Checks, c) }

func (r *run) nested(ctx context.Context, e rawEntry) ([]rawEntry, error) {
	body := []byte(e.Data)
	if e.URL != "" {
		var err error
		if body, err = r.fetch(ctx, e.URL, catalog.MediaTypeCatalog); err != nil {
			return nil, err
		}
	}
	var inner rawCatalog
	if err := json.Unmarshal(body, &inner); err != nil {
		return nil, err
	}
	return inner.Entries, nil
}

// leaf fetches one protocol card, checks its digest, then hands it to the
// protocol checker.
func (r *run) leaf(ctx context.Context, e rawEntry) (Endpoint, bool) {
	var card, hashed []byte
	switch {
	case e.URL != "":
		b, err := r.fetch(ctx, e.URL, e.Type)
		if err != nil {
			r.add(Check{Entry: e.Identifier, ID: "card.fetch", Status: Fail, Want: e.URL, Got: err.Error(), Rule: "AI Catalog entry url"})
			return Endpoint{}, false
		}
		card, hashed = b, b
	case len(e.Data) > 0:
		canon, err := trust.Canonicalize(e.Data)
		if err != nil {
			r.add(Check{Entry: e.Identifier, ID: "card.fetch", Status: Fail, Got: err.Error(), Rule: "AI Catalog entry data (JCS)"})
			return Endpoint{}, false
		}
		card, hashed = e.Data, canon
	default:
		return Endpoint{}, false
	}

	d := Check{Entry: e.Identifier, ID: "entry.digest", Want: e.Digest, Rule: "AI Catalog: Verifying Artifact Integrity, step 4"}
	if e.Digest == "" {
		d.Status, d.Got = Skip, "entry has no digest"
	} else if ok, err := trust.VerifyDigest(e.Digest, hashed); err != nil {
		d.Status, d.Got = Fail, err.Error()
	} else if ok {
		d.Status, d.Got = Pass, e.Digest
	} else {
		d.Status, d.Got = Fail, sha256Of(hashed)
	}
	r.add(d)

	switch e.Type {
	case catalog.MediaTypeMCPServerCard:
		return r.mcp(ctx, e, card), true
	case catalog.MediaTypeA2AAgentCard:
		return r.a2a(ctx, e, card), true
	}
	return Endpoint{Protocol: "other", Identifier: e.Identifier, CardURL: e.URL}, true
}

func (r *run) liveCheck(entry, id, rule string) (Check, bool) {
	c := Check{Entry: entry, ID: id, Rule: rule}
	if !r.opt.Live {
		c.Status, c.Got = Skip, "--no-live"
		r.add(c)
		return c, false
	}
	return c, true
}

func compare(c Check, want, got string) Check {
	c.Want, c.Got = want, got
	c.Status = Pass
	if want != got {
		c.Status = Fail
	}
	return c
}

func (r *run) fetch(ctx context.Context, url, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept+", application/json")
	resp, err := r.opt.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func sha256Of(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func originOf(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		if j := strings.Index(u[i+3:], "/"); j >= 0 {
			return u[:i+3+j]
		}
	}
	return u
}
