package main

import (
	"fmt"
	"os"

	"github.com/ellyZz/suluctl/internal/cmd"
)

// Set via goreleaser ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `suluctl — stream test reports into Sulu TMS

Usage:
  suluctl init     [--framework X] [--package P] [--dry-run] [--force]
  suluctl sync-ids [--framework X] [--package P] [--dir D] [--dry-run] [--force]
  suluctl upload   --results <dir|file|glob> [flags]
  suluctl watch    --results <dir> [flags] -- <test command...>
  suluctl version

Config (flags override env): SULU_URL, SULU_TOKEN, SULU_PROJECT_ID, SULU_LAUNCH_NAME.
Run 'suluctl upload -h' or 'suluctl watch -h' for flags.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "upload":
		return cmd.Upload(args[1:], os.Stdout, os.Stderr, version)
	case "watch":
		return cmd.Watch(args[1:], os.Stdout, os.Stderr, version)
	case "init":
		return cmd.Init(args[1:], os.Stdout, os.Stderr, version)
	case "sync-ids":
		return cmd.SyncIDs(args[1:], os.Stdout, os.Stderr, version)
	case "version", "--version", "-v":
		fmt.Printf("suluctl %s (commit %s, built %s)\n", version, commit, date)
		return 0
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
