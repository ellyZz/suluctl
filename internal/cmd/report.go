package cmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/ellyZz/suluctl/internal/api"
	"github.com/ellyZz/suluctl/internal/config"
	"github.com/ellyZz/suluctl/internal/scan"
	"github.com/ellyZz/suluctl/internal/upload"
)

// newClient is a seam for tests (lets them disable retry backoff sleeps).
var newClient = func(cfg config.Config, version string) *api.Client {
	return api.New(cfg.URL, cfg.Token, cfg.Insecure, version)
}

// isolatableFileError reports whether a request-level error on a SINGLE-file
// batch should fail just that file rather than the run (spec §6 step 3).
// Auth/billing/conflict (401/402/403/404/409) are run-level conditions and are
// never isolated. Retryable-class errors (5xx/429) that survived retries mean a
// server outage — total failure — EXCEPT for a file above the server's per-file
// cap, where the 5xx IS the oversize rejection (Spring's servlet-level
// max-file-size rejects the request before the controller and surfaces as 500).
// Other 4xx (413-class) are file-level rejections. Network errors are never
// isolated (watch keeps retrying them on later ticks).
func isolatableFileError(err error, fileSize int64) bool {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Status {
	case 401, 402, 403, 404, 409:
		return false
	}
	if apiErr.Retryable() {
		return fileSize > upload.SoloFileBytes
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
