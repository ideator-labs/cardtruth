package fixture_test

import (
	"context"
	"net"
	"net/http"
	"slices"
	"sort"
	"testing"

	"github.com/ideator-labs/cardtruth/internal/fixture"
	"github.com/ideator-labs/cardtruth/internal/truth"
)

func serve(t *testing.T, scenario string) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	h, err := fixture.Handler(base, scenario)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return base + "/.well-known/ai-catalog.json"
}

func failed(r *truth.Report) []string {
	var ids []string
	for _, c := range r.Checks {
		if c.Status == truth.Fail {
			ids = append(ids, c.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func TestEachScenarioFailsExactlyItsChecks(t *testing.T) {
	for scenario, want := range fixture.Scenarios {
		t.Run(scenario, func(t *testing.T) {
			rep, err := truth.Run(context.Background(), serve(t, scenario), truth.Options{Live: true})
			if err != nil {
				t.Fatal(err)
			}
			want = slices.Sorted(slices.Values(want))
			if got := failed(rep); !slices.Equal(got, want) {
				t.Fatalf("failed checks = %v, want %v", got, want)
			}
		})
	}
}

func TestStaticChecksMissRuntimeDrift(t *testing.T) {
	for _, scenario := range []string{"stale-mcp-card", "a2a-moved"} {
		rep, err := truth.Run(context.Background(), serve(t, scenario), truth.Options{Live: false})
		if err != nil {
			t.Fatal(err)
		}
		if got := failed(rep); len(got) != 0 {
			t.Fatalf("%s without live checks failed %v; expected the drift to go unseen", scenario, got)
		}
	}
}
