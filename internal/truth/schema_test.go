package truth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServerCardSchemaMatchesUpstreamExamples(t *testing.T) {
	for _, want := range []string{"valid", "invalid"} {
		files, _ := filepath.Glob(filepath.Join("..", "..", "schema", "examples", want, "*.json"))
		if len(files) == 0 {
			t.Fatalf("no %s examples found", want)
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateServerCard(b)
			if want == "valid" && err != nil {
				t.Errorf("%s: want valid, got %v", f, err)
			}
			if want == "invalid" && err == nil {
				t.Errorf("%s: want invalid, schema accepted it", f)
			}
		}
	}
}
