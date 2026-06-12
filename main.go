package main

import (
	"fmt"
	"os"
)

// Set via goreleaser ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `suluctl — stream test reports into Sulu TMS

Usage:
  suluctl upload --results <dir|file|glob> [flags]
  suluctl watch  --results <dir> [flags] -- <test command...>
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
		fmt.Fprintln(os.Stderr, "not implemented yet")
		return 2
	case "watch":
		fmt.Fprintln(os.Stderr, "not implemented yet")
		return 2
	case "version":
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
