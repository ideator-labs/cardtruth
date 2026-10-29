package truth

import (
	"fmt"
	"io"
	"strings"
)

type Status string

const (
	Pass Status = "pass"
	Fail Status = "fail"
	Skip Status = "skip"
)

// Check is one claim compared against one observation.
type Check struct {
	Entry  string `json:"entry"`
	ID     string `json:"id"`
	Status Status `json:"status"`
	Want   string `json:"want,omitempty"`
	Got    string `json:"got,omitempty"`
	Rule   string `json:"rule"`
}

// Endpoint is one protocol access point for a logical agent, as the catalog
// and its card describe it.
type Endpoint struct {
	Protocol   string `json:"protocol"`
	Identifier string `json:"identifier"`
	CardURL    string `json:"cardUrl,omitempty"`
	URL        string `json:"url,omitempty"`
	Name       string `json:"name,omitempty"`
	Version    string `json:"version,omitempty"`
}

// Agent is one top-level catalog entry. A nested catalog entry groups several
// protocol endpoints under one logical agent.
type Agent struct {
	Identifier  string     `json:"identifier"`
	DisplayName string     `json:"displayName,omitempty"`
	Endpoints   []Endpoint `json:"endpoints"`
}

type Report struct {
	Catalog string  `json:"catalog"`
	Live    bool    `json:"live"`
	Agents  []Agent `json:"agents"`
	Checks  []Check `json:"checks"`
}

func (r *Report) Count(s Status) int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == s {
			n++
		}
	}
	return n
}

func (r *Report) WriteText(w io.Writer) {
	fmt.Fprintf(w, "catalog %s (live=%v)\n", r.Catalog, r.Live)
	for _, a := range r.Agents {
		fmt.Fprintf(w, "agent %s\n", a.Identifier)
		for _, e := range a.Endpoints {
			fmt.Fprintf(w, "  %-3s %s -> %s (%s %s)\n", e.Protocol, e.Identifier, e.URL, e.Name, e.Version)
		}
	}
	for _, c := range r.Checks {
		line := fmt.Sprintf("%-4s %-22s %s", strings.ToUpper(string(c.Status)), c.ID, c.Entry)
		if c.Status == Fail {
			line += fmt.Sprintf("  want=%q got=%q", c.Want, c.Got)
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintf(w, "%d pass, %d fail, %d skip\n", r.Count(Pass), r.Count(Fail), r.Count(Skip))
}
