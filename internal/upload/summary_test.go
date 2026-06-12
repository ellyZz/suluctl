package upload

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ellyZz/suluctl/internal/api"
)

func TestSummarize(t *testing.T) {
	ledger := []api.FileResult{
		{FileName: "a-result.json", Status: "PARSED"},
		{FileName: "b-result.json", Status: "PARSED"},
		{FileName: "env.properties", Status: "IGNORED"},
		{FileName: "broken.json", Status: "FAILED", Error: "MALFORMED_JSON"},
		{FileName: "orphan-attachment.png", Status: "UNLINKED"},
		{FileName: "dup-attachment.txt", Status: "DUPLICATE", Error: "DUPLICATE_NAME"},
	}
	responses := []api.FileResult{
		{FileName: "a-result.json", Status: "PARSED"},
		{FileName: "a-result.json", Status: "DUPLICATE"}, // sha-exact re-upload: response-only
		{FileName: "dup-attachment.txt", Status: "DUPLICATE", Error: "DUPLICATE_NAME"},
	}
	local := []api.FileResult{
		{FileName: "giant.bin", Status: "FAILED", Error: "HTTP 413: payload too large"},
	}
	s := Summarize(ledger, responses, local)
	if s.Parsed != 2 || s.Ignored != 1 || s.Unlinked != 1 {
		t.Errorf("ledger counts wrong: %+v", s)
	}
	if s.Duplicate != 2 { // counted from responses (sha-dup + name-dup), not from ledger
		t.Errorf("duplicate: want 2, got %d", s.Duplicate)
	}
	if s.Failed != 2 || len(s.FailedFiles) != 2 { // ledger FAILED + local failure
		t.Errorf("failed: %+v", s)
	}

	var buf bytes.Buffer
	s.Print(&buf)
	out := buf.String()
	for _, want := range []string{"parsed", "broken.json", "MALFORMED_JSON", "giant.bin"} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}
