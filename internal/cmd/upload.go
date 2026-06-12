package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ellyZz/suluctl/internal/api"
	"github.com/ellyZz/suluctl/internal/config"
	"github.com/ellyZz/suluctl/internal/scan"
	"github.com/ellyZz/suluctl/internal/upload"
)

// Upload implements `suluctl upload`. Returns the process exit code:
// 0 = success (even with per-file failures), 1 = total failure, 2 = usage error.
func Upload(args []string, out, errW io.Writer, version string) int {
	cfg := config.FromEnv()
	var results string
	var tags stringList
	envVars := kvMap{}

	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(errW)
	fs.StringVar(&results, "results", "", "results directory, file, or glob (required)")
	fs.StringVar(&cfg.URL, "url", cfg.URL, "Sulu base URL (env SULU_URL)")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "API token (env SULU_TOKEN)")
	fs.Int64Var(&cfg.ProjectID, "project", cfg.ProjectID, "project id (env SULU_PROJECT_ID)")
	fs.StringVar(&cfg.LaunchName, "launch-name", cfg.LaunchName, "launch name (env SULU_LAUNCH_NAME)")
	fs.StringVar(&cfg.Environment, "env", "", "launch environment")
	fs.Var(&tags, "tag", "launch tag (repeatable)")
	fs.Var(&envVars, "env-var", "launch env var K=V (repeatable)")
	fs.BoolVar(&cfg.Insecure, "insecure", false, "skip TLS certificate verification")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
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

	files, err := scan.Collect(results)
	if err != nil {
		fmt.Fprintln(errW, err)
		return 1
	}

	client := newClient(cfg, version)
	client.Logf = func(format string, a ...any) { fmt.Fprintf(errW, format+"\n", a...) }

	session, err := client.CreateLaunch(api.LaunchRequest{
		ProjectID:   cfg.ProjectID,
		Name:        cfg.LaunchName,
		Environment: cfg.Environment,
		Tags:        cfg.Tags,
		EnvVars:     cfg.EnvVars,
	})
	if err != nil {
		fmt.Fprintf(errW, "creating launch failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "launch %d: uploading %d files\n", session.LaunchID, len(files))

	var responses, local []api.FileResult
	totalFailure := false
	for _, batch := range upload.Batches(files) {
		res, err := client.UploadFiles(session.LaunchUUID, paths(batch))
		if err != nil {
			if len(batch) == 1 && isolatableFileError(err, batch[0].Size) {
				// per-file isolation: a request-level rejection of a single
				// (oversize) file fails that file only, not the run (spec §6)
				fmt.Fprintf(errW, "file %s rejected: %v\n", batch[0].Path, err)
				local = append(local, api.FileResult{
					FileName: filepath.Base(batch[0].Path), Status: "FAILED", Error: err.Error()})
				continue
			}
			// auth/402/409 or exhausted retries — total failure (spec §4);
			// 409 additionally means the session is closed, so stop uploading
			fmt.Fprintf(errW, "upload failed: %v\n", err)
			totalFailure = true
			break
		}
		responses = append(responses, res...)
	}

	return finishAndReport(client, session, responses, local, "", totalFailure, out, errW, cfg.URL)
}
