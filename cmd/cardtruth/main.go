// Command cardtruth checks that an AI Catalog, the MCP Server Cards and A2A
// Agent Cards it lists, and the live endpoints behind them agree.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ideator-labs/cardtruth/internal/truth"
)

func main() {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	noLive := fs.Bool("no-live", false, "skip live endpoint checks (catalog, digest and schema only)")
	jsonOut := fs.String("json", "", "also write the report as JSON to this file")
	if len(os.Args) < 3 || os.Args[1] != "check" {
		fmt.Fprintln(os.Stderr, "usage: cardtruth check [--no-live] [--json report.json] <catalog-url | domain-url>")
		os.Exit(2)
	}
	fs.Parse(os.Args[2:])
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "need exactly one catalog URL")
		os.Exit(2)
	}
	url := fs.Arg(0)
	if !strings.HasSuffix(url, ".json") {
		url = strings.TrimSuffix(url, "/") + "/.well-known/ai-catalog.json"
	}

	rep, err := truth.Run(context.Background(), url, truth.Options{Live: !*noLive})
	if err != nil {
		fmt.Fprintln(os.Stderr, "cardtruth:", err)
		os.Exit(2)
	}
	rep.WriteText(os.Stdout)
	if *jsonOut != "" {
		b, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(*jsonOut, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "cardtruth:", err)
			os.Exit(2)
		}
	}
	if rep.Count(truth.Fail) > 0 {
		os.Exit(1)
	}
}
