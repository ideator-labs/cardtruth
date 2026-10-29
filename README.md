# CardTruth

Does what your AI Catalog says match what your agent actually serves?

CardTruth starts from a domain's `/.well-known/ai-catalog.json`, follows each entry to its MCP Server Card or A2A Agent Card, and then connects to the live endpoint the card names. It reports every place where the catalog, the card, and the running server disagree.

```
go run ./cmd/cardtruth check https://example.com
go run ./cmd/cardtruth check --no-live --json report.json https://example.com/.well-known/ai-catalog.json
```

Exit 0 means every check passed. Exit 1 means at least one check failed. Exit 2 means the catalog could not be read.

## Why

MCP Server Cards (SEP-2127, merged 2026-10-06) and A2A Agent Cards are both discovered through the same [AI Catalog](https://ai-catalog.io/). The Server Card discovery doc says a card's claims are advisory and that clients SHOULD verify them against the live connection. Catalog validators check the document, not the server. A card that went stale after a deploy still validates.

## Proof

`bash demo.sh` runs a loopback publisher with a real MCP server (official Go SDK v1.8.0) and a real A2A agent (a2a-go v2.7.0) through five scenarios. It passes 10/10. `bash demo.sh --break` turns live checks off and fails 5 of them, which is the point: static checks miss runtime drift.

Tool and agent replies in the fixture are stubs. No model is called.

## For agents and contributors

Start with [AGENTS.md](AGENTS.md). It has the data shape, repo map, verified run steps, limits, and next steps.

License: Apache-2.0.
