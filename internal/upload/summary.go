package upload

import (
	"fmt"
	"io"

	"github.com/ellyZz/suluctl/internal/api"
)

// Summary aggregates the post-finish ledger (authoritative; the only place
// UNLINKED appears) with the per-request upload responses (the only place
// pure-sha256 duplicates appear — they never get ledger rows) and local
// request-level failures (oversize single-file batches rejected by the server);
// local rows are always counted as failures regardless of their Status field.
// Note: a lost-response retry can legitimately make Parsed+Duplicate exceed the local file count (both are real server events).
type Summary struct {
	Parsed, Duplicate, Ignored, Failed, Unlinked, PendingLink int
	FailedFiles                                               []api.FileResult
}

func Summarize(ledger, responses, local []api.FileResult) Summary {
	var s Summary
	for _, r := range ledger {
		switch r.Status {
		case "PARSED":
			s.Parsed++
		case "IGNORED":
			s.Ignored++
		case "FAILED":
			s.Failed++
			s.FailedFiles = append(s.FailedFiles, r)
		case "UNLINKED":
			s.Unlinked++
		case "PENDING_LINK":
			s.PendingLink++
		}
		// ledger DUPLICATE rows (name-dups) are deliberately not counted here:
		// they also appear in the upload responses, which we count below.
	}
	for _, r := range responses {
		if r.Status == "DUPLICATE" {
			s.Duplicate++
		}
	}
	for _, r := range local {
		s.Failed++
		s.FailedFiles = append(s.FailedFiles, r)
	}
	return s
}

func (s Summary) Print(w io.Writer) {
	fmt.Fprintf(w, "  parsed     %d\n", s.Parsed)
	fmt.Fprintf(w, "  duplicate  %d\n", s.Duplicate)
	fmt.Fprintf(w, "  ignored    %d\n", s.Ignored)
	fmt.Fprintf(w, "  failed     %d\n", s.Failed)
	fmt.Fprintf(w, "  unlinked   %d\n", s.Unlinked)
	if s.PendingLink > 0 {
		fmt.Fprintf(w, "  pending    %d\n", s.PendingLink)
	}
	if len(s.FailedFiles) > 0 {
		fmt.Fprintln(w, "\nfailed files:")
		for _, f := range s.FailedFiles {
			fmt.Fprintf(w, "  %s  %s\n", f.FileName, f.Error)
		}
	}
}
