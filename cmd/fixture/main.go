// Command fixture serves one drift scenario on a loopback port and writes its
// base URL to --url-file.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/ideator-labs/cardtruth/internal/fixture"
)

func main() {
	scenario := flag.String("scenario", "honest", "honest | stale-mcp-card | a2a-moved | card-edited | protocol-overclaim")
	urlFile := flag.String("url-file", "", "write the base URL here once listening")
	flag.Parse()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	h, err := fixture.Handler(base, *scenario)
	if err != nil {
		log.Fatal(err)
	}
	if *urlFile != "" {
		if err := os.WriteFile(*urlFile, []byte(base), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("fixture %s on %s", *scenario, base)
	log.Fatal(http.Serve(ln, h))
}
