# AGENTS.md: CardTruth

Handoff for a coding agent with no prior context.

## What this is and the claim

CardTruth is a Go CLI. It reads an AI Catalog, fetches every MCP Server Card and A2A Agent Card the catalog lists, and checks each card against the live endpoint it points to.

Claim: a catalog and its cards can all pass static validation while the live server contradicts them. The official ai-catalog-go validator and schema checks accept all five drift scenarios in the demo. Only the live checks catch the three runtime ones.

Why it matters for adoption. AI Catalog is the shared discovery layer that both MCP (SEP-2127 Server Cards) and A2A point to. Every publisher that passes a shared live check becomes safe for every MCP and A2A client to auto-configure. Every registry or crawler that runs the same check raises the floor for all publishers. That compounding only works if the check is shared, which is why this should live next to the specs and not inside one client.

## Status

Real:
- AI Catalog parsing and validation use `github.com/Agent-Card/ai-catalog-go` v0.3.0 (`catalog`, `validate`, `trust`).
- MCP live checks use the official `github.com/modelcontextprotocol/go-sdk` v1.8.0 client over Streamable HTTP. It tries `server/discover` first and falls back to `initialize`.
- A2A live checks use `github.com/a2aproject/a2a-go/v2` v2.7.0 (`agentcard.Resolver`, `a2aclient.NewFromCard`, `SendMessage` over JSON-RPC).
- Server Card schema validation uses the vendored SEP-2127 `schema.json` with `github.com/google/jsonschema-go` v0.4.3.

Stubbed:
- The fixture's MCP tool `quote` and the A2A agent return fixed text. No model is called.
- The fixture publisher is loopback HTTP only. No real public catalog was checked.

## Data shape

`internal/truth/report.go` is the contract.

```
Report  { catalog, live, agents[], checks[] }
Agent   { identifier, displayName, endpoints[] }   one top-level catalog entry
Endpoint{ protocol: mcp|a2a|other, identifier, cardUrl, url, name, version }
Check   { entry, id, status: pass|fail|skip, want, got, rule }
```

Check IDs:

| id | compares | rule |
| --- | --- | --- |
| catalog.sdk-valid | catalog vs ai-catalog-go `validate.Validate` | SDK |
| entry.digest | `entry.digest` vs sha256 of fetched card bytes (or JCS of inline data) | AI Catalog, Verifying Artifact Integrity |
| mcp.card.schema | card vs SEP-2127 `#/$defs/ServerCard` | SEP-2127 |
| mcp.live.connect | first concrete `streamable-http` remote accepts a connection | go-sdk |
| mcp.live.name / mcp.live.version | card `name`/`version` vs live `serverInfo` | Server Card discovery.md, Consistency with Runtime Behavior |
| mcp.live.protocol | negotiated protocol version is in `remotes[].supportedProtocolVersions` | same |
| a2a.card.parse | card decodes as A2A 1.0 AgentCard with name, version, supportedInterfaces | A2A 1.0 |
| a2a.live.card | catalog card name@version and interface URL vs the agent's own `/.well-known/agent-card.json` | A2A discovery |
| a2a.live.send | `SendMessage` to `supportedInterfaces[0]` gets a reply | a2a-go client |

`--no-live` marks every live check `skip`.

## Repo map

- `cmd/cardtruth/main.go` is the CLI. A URL not ending in `.json` gets `/.well-known/ai-catalog.json` appended.
- `cmd/fixture/main.go` serves one scenario on `127.0.0.1:0` and writes its base URL to `--url-file`.
- `internal/truth/truth.go` walks the catalog, follows one level of nested catalogs, fetches cards, checks digests.
- `internal/truth/mcp.go` runs schema and live MCP checks.
- `internal/truth/a2a.go` runs A2A parse and live checks.
- `internal/fixture/fixture.go` builds the publisher: catalog, Server Card, MCP server, A2A card and agent. `Scenarios` maps each scenario to the checks it must fail.
- `internal/fixture/fixture_test.go` asserts that scenario matrix, and that `--no-live` misses the runtime drift.
- `internal/truth/schema_test.go` runs the upstream valid and invalid Server Card examples against the vendored schema.
- `schema/` holds the vendored schema and examples. See `schema/README.md` for the source commit.
- `demo.sh` is the proof artifact.

## Setup, run, verify (fresh clone)

Needs Go 1.27.3 or newer (`go.mod` says `go 1.27.3`), `jq`, and network access to the Go module proxy on first build.

```
git clone https://github.com/ideator-labs/cardtruth && cd cardtruth
go test ./...
bash demo.sh            # expect "10/10 checks passed", exit 0
bash demo.sh --break    # expect "5/10 checks passed", exit 1
```

Expected scenario lines from `bash demo.sh`:

```
--- honest: 11 pass, 0 fail, 0 skip, exit 0
--- stale-mcp-card: 10 pass, 1 fail, 0 skip, exit 1
--- a2a-moved: 9 pass, 2 fail, 0 skip, exit 1
--- card-edited: 10 pass, 1 fail, 0 skip, exit 1
--- protocol-overclaim: 10 pass, 1 fail, 0 skip, exit 1
```

Manual run against one scenario:

```
go run ./cmd/fixture --scenario stale-mcp-card --url-file /tmp/u &
go run ./cmd/cardtruth check "$(cat /tmp/u)"
kill %1
```

## Scenarios

- `honest`: everything agrees. The stateless go-sdk handler negotiates MCP 2026-07-28.
- `stale-mcp-card`: the MCP server reports 1.3.0, the card still says 1.2.0.
- `a2a-moved`: the agent moved to `/a2a/invoke`. The catalog's copy of the Agent Card still names `/a2a/v1/invoke`, which returns 404.
- `card-edited`: the Server Card's description changed after the catalog digest was computed.
- `protocol-overclaim`: the card lists only MCP 2026-07-28. The server uses go-sdk's stateful Streamable HTTP handler, which does not serve 2026-07-28 in v1.8.0, so the client falls back to `initialize` at 2025-11-25. This was measured, not designed. It is a real way to ship a card that overclaims.

## Design decisions

- Entry members are read from raw JSON as well as through the SDK. ai-catalog-go v0.3.0 `CatalogEntry` has no `digest` field, while the spec at Agent-Card/ai-catalog commit 9aac2c1 defines one.
- One logical agent per top-level entry. A nested catalog entry becomes one agent with several endpoints, following the spec's multi-protocol example.
- MCP identity comes from `InitializeResult().ServerInfo`, which go-sdk fills from `server/discover` or `initialize`. No hand-rolled JSON-RPC.
- The A2A well-known card is resolved at the origin of `supportedInterfaces[0].url`, since that is where the agent itself lives.
- Exit codes are the interface for CI: 0 clean, 1 drift, 2 unreadable.

## Known limits

- Only one level of nested catalogs is followed.
- Only the first concrete `streamable-http` remote is probed. URL templates (`{var}`) and `sse` remotes are skipped.
- No Trust Manifest signature checks. `entry.digest` proves consistency with the catalog, not who published it.
- No SSRF guard. Do not point it at untrusted catalogs from a server with internal network access.
- A2A checks speak JSON-RPC only. gRPC and HTTP+JSON interfaces are not tried.
- Live identity can legitimately differ for multi-tenant or templated servers. There is no allowlist for expected differences yet.
- Not yet run against a real public catalog.

## Next steps (in priority order)

1. Run against public catalogs. Done when `cardtruth check` has been run on at least three real `/.well-known/ai-catalog.json` URLs and the results are recorded in `docs/field-results.md`.
2. Trust Manifest verification. Done when a signed manifest from the tomevault-io reference vectors passes and a tampered one fails a new `entry.trust` check.
3. Nested depth and cycles. Done when a test catalog three levels deep resolves and a self-referencing catalog stops with a `catalog.nested` fail instead of looping.
4. Agent Plugins entries. Done when an `application/agent-plugins+zip` entry is downloaded, its digest checked, and its `mcp.json` remotes probed like Server Card remotes.
5. GitHub Action. Done when a workflow in a sample repo fails a PR whose catalog drifts from a deployed preview URL.
6. SSRF-safe fetch. Done when loopback, link-local and private addresses are refused unless `--allow-private` is set, with a test for each.

## Working rules

- Commit as `danielgerlag <daniel@gerlag.ca>`.
- `bash demo.sh` must stay green and `bash demo.sh --break` must stay red. Add a scenario to `fixture.Scenarios` for every new check.
- Keep stubs labelled as stubs. Never call a paid model API.
- Short sentences, no em-dashes, no invented links.

## Context and sources

- SEP-2127 MCP Server Cards: https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2127 (merged 2026-10-06)
- Server Card extension and discovery doc: https://github.com/modelcontextprotocol/experimental-ext-server-card
- AI Catalog spec: https://ai-catalog.io/ and https://github.com/Agent-Card/ai-catalog
- AI Catalog Go SDK: https://github.com/agntcy/ai-catalog-go-sdk
- A2A 1.0 specification: https://a2a-protocol.org/latest/specification/
- a2a-go: https://github.com/a2aproject/a2a-go
- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- AAIF projects (MCP and A2A are both hosted): https://aaif.io/projects
