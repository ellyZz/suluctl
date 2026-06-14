package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ellyZz/suluctl/internal/config"
	"github.com/ellyZz/suluctl/internal/initscaffold"
	"github.com/ellyZz/suluctl/internal/syncids"
)

// SyncIDs implements `suluctl sync-ids`. Returns 0 success, 1 operational failure, 2 usage error.
func SyncIDs(args []string, out, errW io.Writer, version string) int {
	cfg := config.FromEnv()
	var framework, pkg, dir string
	var dryRun, force bool

	fs := flag.NewFlagSet("sync-ids", flag.ContinueOnError)
	fs.SetOutput(errW)
	fs.StringVar(&cfg.URL, "url", cfg.URL, "Sulu base URL (env SULU_URL)")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "API token (env SULU_TOKEN)")
	fs.Int64Var(&cfg.ProjectID, "project", cfg.ProjectID, "project id (env SULU_PROJECT_ID)")
	fs.StringVar(&framework, "framework", "", "force framework: testng|junit5|pytest|playwright|xunit (default: autodetect)")
	fs.StringVar(&pkg, "package", "", "Java base package (default: detected)")
	fs.StringVar(&dir, "dir", "", "directory to scan (default: cwd)")
	fs.BoolVar(&dryRun, "dry-run", false, "report the plan; write nothing")
	fs.BoolVar(&force, "force", false, "overwrite an existing id even if it differs from the resolved displayId")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if framework != "" && !knownSyncKind(framework) {
		fmt.Fprintf(errW, "unknown --framework %q (supported: testng, junit5, pytest, playwright, xunit)\n", framework)
		return 2
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(errW, err)
		return 2
	}

	root := dir
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(errW, err)
			return 1
		}
		root = wd
	}

	var kind initscaffold.Kind
	if framework != "" {
		kind = initscaffold.Kind(framework)
	} else {
		k, derr := initscaffold.Detect(root)
		if derr != nil {
			fmt.Fprintln(errW, derr)
			return 1
		}
		kind = k
	}

	opts := syncids.Options{}
	switch string(kind) {
	case "testng", "junit5":
		if pkg == "" {
			pkg = syncids.ResolveJavaImportPackage(root) // package of the project's SuluTest.java
		}
		if pkg == "" {
			pkg = initscaffold.DetectJavaBasePackage(root)
		}
		opts.JavaPackage = pkg
	case "playwright":
		opts.TestDir = syncids.DetectPlaywrightTestDir(root)
	}

	client := newClient(cfg, version)
	res, err := syncids.Run(syncids.Config{
		Client: client, ProjectID: cfg.ProjectID, Kind: kind,
		Dir: root, Opts: opts, DryRun: dryRun, Force: force,
	})
	if err != nil {
		fmt.Fprintf(errW, "sync-ids failed: %v\n", err)
		return 1
	}
	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}
	fmt.Fprintf(out, "%ssync-ids (%s): %d written, %d already set, %d not found, %d resolved, %d errors\n",
		prefix, kind, res.Written, res.AlreadySet, res.NotFound, res.Resolved, res.Errors)
	if res.Errors > 0 {
		return 1
	}
	return 0
}

func knownSyncKind(s string) bool {
	switch s {
	case "testng", "junit5", "pytest", "playwright", "xunit":
		return true
	}
	return false
}
