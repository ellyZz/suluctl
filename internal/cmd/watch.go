package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ellyZz/suluctl/internal/api"
	"github.com/ellyZz/suluctl/internal/config"
	"github.com/ellyZz/suluctl/internal/console"
	"github.com/ellyZz/suluctl/internal/runner"
	"github.com/ellyZz/suluctl/internal/scan"
	"github.com/ellyZz/suluctl/internal/upload"
)

// watchTick is a variable so tests can speed the polling loop up.
var watchTick = 2 * time.Second

// Watch implements `suluctl watch ... -- <cmd>`. Always exits with the wrapped
// command's exit code; Sulu being unreachable only degrades streaming.
func Watch(args []string, out, errW io.Writer, version string) int {
	cliArgs, childArgv := splitOnDashDash(args)
	helpRequested := false
	for _, a := range cliArgs {
		if a == "-h" || a == "--help" {
			helpRequested = true
		}
	}
	if len(childArgv) == 0 && !helpRequested {
		fmt.Fprintln(errW, "usage: suluctl watch --results <dir> [flags] -- <test command...>")
		return 2
	}

	cfg := config.FromEnv()
	var results string
	var clean bool
	var tags stringList
	envVars := kvMap{}
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(errW)
	fs.StringVar(&results, "results", "", "results directory to watch (required)")
	fs.StringVar(&cfg.URL, "url", cfg.URL, "Sulu base URL (env SULU_URL)")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "API token (env SULU_TOKEN)")
	fs.Int64Var(&cfg.ProjectID, "project", cfg.ProjectID, "project id (env SULU_PROJECT_ID)")
	fs.StringVar(&cfg.LaunchName, "launch-name", cfg.LaunchName, "launch name (env SULU_LAUNCH_NAME)")
	fs.StringVar(&cfg.Environment, "env", "", "launch environment")
	fs.Var(&tags, "tag", "launch tag (repeatable)")
	fs.Var(&envVars, "env-var", "launch env var K=V (repeatable)")
	fs.BoolVar(&cfg.Insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&clean, "clean", false, "empty the results dir before starting the command")
	fs.BoolVar(&cfg.ShipConsole, "ship-console", cfg.ShipConsole,
		"ship the wrapped command's console output to Sulu as launch-scoped logs (env SULU_SHIP_CONSOLE; default true)")
	if err := fs.Parse(cliArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if len(childArgv) == 0 {
		fmt.Fprintln(errW, "usage: suluctl watch --results <dir> [flags] -- <test command...>")
		return 2
	}
	cfg.Tags, cfg.EnvVars = tags, envVars
	if results == "" {
		fmt.Fprintln(errW, "--results is required")
		return 2
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(errW, err)
		return 2
	}
	if cfg.Insecure {
		fmt.Fprintln(errW, "WARNING: --insecure skips TLS certificate verification (MITM risk); prefer adding your CA to the system trust store")
	}
	if clean {
		if err := cleanDir(results); err != nil {
			fmt.Fprintf(errW, "--clean failed: %v\n", err)
			return 2
		}
	}

	client := newClient(cfg, version)
	// Logf deliberately left nil for watch: per-attempt retry logs would spam
	// stderr on every 2 s tick during a Sulu outage; the failStreak warning
	// below is the rate-limited signal the spec (§7.5) requires.

	session, err := client.CreateLaunch(api.LaunchRequest{
		ProjectID:   cfg.ProjectID,
		Name:        cfg.LaunchName,
		Environment: cfg.Environment,
		Tags:        cfg.Tags,
		EnvVars:     cfg.EnvVars,
	})
	if err != nil {
		// fail-safe (spec §7): the test run must never be blocked by Sulu
		fmt.Fprintf(errW, "WARNING: cannot create Sulu launch (%v) — running command without streaming\n", err)
		code, _, startErr := runner.Run(childArgv)
		if startErr != nil {
			fmt.Fprintf(errW, "failed to start command: %v\n", startErr)
		}
		return mapExitCode(code)
	}
	fmt.Fprintf(errW, "streaming %s into launch %d\n", results, session.LaunchID)

	scanner := scan.NewScanner(results)
	type exitResult struct {
		code        int
		interrupted bool
		err         error
	}
	exitCh := make(chan exitResult, 1)
	var capr *console.Capturer
	var capOut, capErr *console.LineWriter
	var childOut, childErr io.Writer // nil => runner inherits os.Stdout/os.Stderr
	if cfg.ShipConsole {
		capr = console.New()
		capOut = capr.Writer("INFO")
		capErr = capr.Writer("ERROR")
		childOut = io.MultiWriter(os.Stdout, capOut)
		childErr = io.MultiWriter(os.Stderr, capErr)
	}
	go func() {
		code, intr, err := runner.RunWithCapture(childArgv, childOut, childErr)
		exitCh <- exitResult{code, intr, err}
	}()

	var responses, local []api.FileResult
	failStreak := 0
	sessionClosed := false
	uploadReady := func(files []scan.FileState) {
		if sessionClosed {
			return
		}
		for _, batch := range upload.Batches(files) {
			res, err := client.UploadFiles(session.LaunchUUID, paths(batch))
			if err != nil {
				if len(batch) == 1 && isolatableFileError(err, batch[0].Size) {
					// oversize/413-class rejection of one file: record and don't
					// retry unless the file changes on disk
					local = append(local, api.FileResult{
						FileName: filepath.Base(batch[0].Path), Status: "FAILED", Error: err.Error()})
					scanner.MarkSent(batch)
					continue
				}
				var apiErr *api.APIError
				if errors.As(err, &apiErr) && apiErr.Status == 409 {
					// IMPORT_LAUNCH_FINISHED — the session is closed, stop streaming
					sessionClosed = true
					fmt.Fprintf(errW, "WARNING: import session is closed (%v) — streaming stopped\n", err)
					return
				}
				// network / 5xx-after-retries / auth: leave the batch UNSENT so the
				// next tick (and the final sweep) retries it — a transient Sulu
				// outage must not drop files (spec §7.5)
				if failStreak == 0 {
					fmt.Fprintf(errW, "WARNING: upload failed (%v) — will keep retrying\n", err)
				}
				failStreak++
				return
			}
			failStreak = 0
			responses = append(responses, res...)
			scanner.MarkSent(batch)
		}
	}

	scanFailStreak := 0
	doScan := func(scanFn func() ([]scan.FileState, error)) {
		files, err := scanFn()
		if err != nil {
			if scanFailStreak == 0 {
				fmt.Fprintf(errW, "WARNING: cannot scan %s (%v) — will keep trying\n", results, err)
			}
			scanFailStreak++
			return
		}
		scanFailStreak = 0
		if len(files) > 0 {
			uploadReady(files)
		}
	}

	ticker := time.NewTicker(watchTick)
	defer ticker.Stop()
	var res exitResult
loop:
	for {
		select {
		case <-ticker.C:
			doScan(scanner.Scan)
		case res = <-exitCh:
			break loop
		}
	}
	if res.err != nil {
		fmt.Fprintf(errW, "failed to start command: %v\n", res.err)
	}

	// final sweep: the writer exited, stability is moot
	doScan(scanner.SweepAll)
	state := ""
	if res.interrupted {
		state = "STOPPED"
	}
	if cfg.ShipConsole && capr != nil {
		capOut.Flush()
		capErr.Flush()
		shipConsole(client, session.LaunchID, capr, errW)
	}
	finishAndReport(client, session, responses, local, state, false, out, errW, cfg.URL)
	return mapExitCode(res.code)
}

// mapExitCode translates the runner's signal-death sentinel (-1) into the
// conventional 130 — os.Exit on a negative value would wrap to 255.
func mapExitCode(code int) int {
	if code < 0 {
		return 130
	}
	return code
}

// splitOnDashDash splits args at the first "--" into (cli flags, child argv).
func splitOnDashDash(args []string) (cli, child []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

// shipConsole best-effort posts the captured console as launch-scoped logs.
// Failures are warnings only — they never affect the wrapped command's exit code.
func shipConsole(client *api.Client, launchID int64, capr *console.Capturer, errW io.Writer) {
	if capr.Truncated() {
		fmt.Fprintln(errW, "WARNING: console output exceeded the capture cap — shipped logs are truncated")
	}
	entries := toLogEntries(capr.Entries())
	if len(entries) == 0 {
		return
	}
	if err := client.AppendLaunchLogs(launchID, entries); err != nil {
		fmt.Fprintf(errW, "WARNING: shipping console logs failed (%v)\n", err)
	}
}

func toLogEntries(in []console.Entry) []api.LogEntry {
	out := make([]api.LogEntry, len(in))
	for i, e := range in {
		out[i] = api.LogEntry{
			Timestamp: e.TS.Format("2006-01-02T15:04:05.000"),
			Level:     e.Level,
			Message:   e.Message,
			Source:    "suluctl-console",
		}
	}
	return out
}

// cleanDir removes the contents of dir (not dir itself). Missing dir is a no-op.
// Refuses filesystem root and the user's home directory outright — a typo'd
// --results with --clean must not become rm -rf equivalent.
func cleanDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if abs == string(filepath.Separator) || abs == filepath.VolumeName(abs)+string(filepath.Separator) {
		return fmt.Errorf("refusing to clean filesystem root %q", abs)
	}
	if home, herr := os.UserHomeDir(); herr == nil && abs == home {
		return fmt.Errorf("refusing to clean home directory %q", abs)
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
