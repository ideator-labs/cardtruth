// Package schema embeds the MCP Server Card JSON Schema, vendored from
// modelcontextprotocol/experimental-ext-server-card at commit 526201bb
// (the snapshot SEP-2127 was accepted against).
package schema

import _ "embed"

//go:embed server-card.schema.json
var ServerCard []byte
