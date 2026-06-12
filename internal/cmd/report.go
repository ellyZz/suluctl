package cmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/ellyZz/suluctl/internal/api"
	"github.com/ellyZz/suluctl/internal/scan"
	"github.com/ellyZz/suluctl/internal/upload"
)

// isolatableFileError reports whether a request-level error on a SINGLE-file
// batch should fail just that file rather than the run (spec §6): the
// oversize/413-class 4xx rejections. Auth/billing/conflict (401/402/403/404/409)
// are run-level conditions, and network/5xx-after-retries must stay retryable
// (watch re-sends on later ticks) — none of those are isolated.
func isolatableFileError(err error) bool {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return false // network-class error
	}
	switch apiErr.Status {
	case 401, 402, 403, 404, 409:
		return false
	}
	return apiErr.Status >= 400 && apiErr.Status < 500
}

// finishAndReport finishes the session, fetches the authoritative ledger and
// prints the summary + launch link. Returns 1 when totalFailure (for `upload`;
// `watch` ignores the return and always exits with the child's code).
func finishAndReport(client *api.Client, session api.LaunchSession,
	responses, local []api.FileResult, executionState string, totalFailure bool,
	out, errW io.Writer, baseURL string) int {

	if err := client.Finish(session.LaunchUUID, executionState); err != nil {
		fmt.Fprintf(errW, "finish failed: %v\n", err)
		totalFailure = true
	}
	ledger, err := client.Ledger(session.LaunchUUID)
	if err != nil {
		fmt.Fprintf(errW, "ledger fetch failed (%v) — summary from upload responses only\n", err)
		ledger = responses
	}
	upload.Summarize(ledger, responses, local).Print(out)
	fmt.Fprintf(out, "\nLaunch: %s/app/launches/%d\n", baseURL, session.LaunchID)
	if totalFailure {
		return 1
	}
	return 0
}

func paths(batch []scan.FileState) []string {
	out := make([]string, len(batch))
	for i, f := range batch {
		out[i] = f.Path
	}
	return out
}
