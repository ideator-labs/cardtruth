#!/usr/bin/env bash
# Proves: catalog and card validation alone miss drift between an AI Catalog,
# its MCP Server Card / A2A Agent Card, and the live endpoints; cardtruth's
# live checks catch each kind. `bash demo.sh --break` turns live checks off
# and must fail.
set -uo pipefail
cd "$(dirname "$0")"

LIVE_FLAG=""
if [[ "${1:-}" == "--break" ]]; then
  LIVE_FLAG="--no-live"
  echo "BREAK MODE: live checks disabled, drift checks are expected to FAIL"
fi

OUT=$(mktemp -d)
trap 'kill $(jobs -p) 2>/dev/null; rm -rf "$OUT"' EXIT

go build -o "$OUT/cardtruth" ./cmd/cardtruth || exit 2
go build -o "$OUT/fixture" ./cmd/fixture || exit 2

pass=0; fail=0
check() {
  if [[ "$2" == "true" ]]; then echo "PASS $1"; pass=$((pass+1)); else echo "FAIL $1"; fail=$((fail+1)); fi
}

run() {
  local scenario=$1 tag=$2 flags=$3
  "$OUT/fixture" --scenario "$scenario" --url-file "$OUT/$tag.url" 2>"$OUT/$tag.fixture.log" &
  local pid=$!
  for _ in $(seq 100); do [[ -s "$OUT/$tag.url" ]] && break; sleep 0.05; done
  "$OUT/cardtruth" check $flags --json "$OUT/$tag.json" "$(cat "$OUT/$tag.url")" >"$OUT/$tag.txt"
  echo $? >"$OUT/$tag.exit"
  kill $pid; wait $pid 2>/dev/null
  echo "--- $tag: $(tail -1 "$OUT/$tag.txt"), exit $(cat "$OUT/$tag.exit")"
}

fails() { jq -r '[.checks[] | select(.status=="fail") | .id] | sort | join(",")' "$OUT/$1.json"; }
got() { jq -r --arg id "$2" '.checks[] | select(.id==$id) | .got' "$OUT/$1.json" | head -1; }

for s in honest stale-mcp-card a2a-moved card-edited protocol-overclaim; do run "$s" "$s" "$LIVE_FLAG"; done
for s in stale-mcp-card a2a-moved protocol-overclaim; do run "$s" "$s-static" "--no-live"; done

echo
check "1. honest publisher: every check passes, exit 0" \
  "$([[ $(cat $OUT/honest.exit) == 0 && $(jq '[.checks[]|select(.status=="pass")]|length' $OUT/honest.json) == 11 ]] && echo true)"
check "2. one lookup yields one agent with an MCP and an A2A endpoint" \
  "$([[ $(jq -r '[.agents[0].endpoints[].protocol]|join(",")' $OUT/honest.json) == "mcp,a2a" ]] && echo true)"
check "3. MCP server upgraded, card stale: only mcp.live.version fails (card 1.2.0, live $(got stale-mcp-card mcp.live.version))" \
  "$([[ $(fails stale-mcp-card) == "mcp.live.version" && $(cat $OUT/stale-mcp-card.exit) == 1 ]] && echo true)"
check "4. A2A agent moved, catalog card stale: a2a.live.card and a2a.live.send fail" \
  "$([[ $(fails a2a-moved) == "a2a.live.card,a2a.live.send" && $(cat $OUT/a2a-moved.exit) == 1 ]] && echo true)"
check "5. Server Card edited after the catalog was published: only entry.digest fails" \
  "$([[ $(fails card-edited) == "entry.digest" ]] && echo true)"
check "6. card claims only MCP 2026-07-28, stateful server negotiates $(got protocol-overclaim mcp.live.protocol): only mcp.live.protocol fails" \
  "$([[ $(fails protocol-overclaim) == "mcp.live.protocol" ]] && echo true)"
check "7. control: ai-catalog-go validate.Validate accepts the catalog in all five scenarios" \
  "$([[ $(cat $OUT/{honest,stale-mcp-card,a2a-moved,card-edited,protocol-overclaim}.json | jq -s '[.[].checks[]|select(.id=="catalog.sdk-valid" and .status=="pass")]|length') == 5 ]] && echo true)"
check "8. control: static checks alone (--no-live) pass all three runtime-drift scenarios" \
  "$([[ $(cat $OUT/stale-mcp-card-static.exit) == 0 && $(cat $OUT/a2a-moved-static.exit) == 0 && $(cat $OUT/protocol-overclaim-static.exit) == 0 ]] && echo true)"
check "9. honest MCP connection negotiated $(got honest mcp.live.protocol), a version the card lists" \
  "$([[ $(jq -r '.checks[]|select(.id=="mcp.live.protocol")|.status' $OUT/honest.json) == pass ]] && echo true)"
check "10. go test ./... (schema vs upstream examples, scenario matrix)" \
  "$(go test ./... >"$OUT/gotest.log" 2>&1 && echo true)"

echo
echo "$pass/$((pass+fail)) checks passed"
[[ $fail == 0 ]]
