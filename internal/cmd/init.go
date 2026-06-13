package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/ellyZz/suluctl/internal/config"
	"github.com/ellyZz/suluctl/internal/initscaffold"
)

// Init implements `suluctl init`. Returns 0 success, 1 operational failure, 2 usage error.
func Init(args []string, out, errW io.Writer, version string) int {
	cfg := config.FromEnv()
	var framework, pkg string
	var dryRun, force bool

	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(errW)
	fs.StringVar(&framework, "framework", "", "force framework: testng|junit5|playwright|pytest|xunit (default: autodetect)")
	fs.StringVar(&pkg, "package", "", "Java base package for the glue (default: detected, else 'sulu')")
	fs.BoolVar(&dryRun, "dry-run", false, "print the plan; write nothing")
	fs.BoolVar(&force, "force", false, "overwrite managed glue files")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if framework != "" && !knownKind(framework) {
		fmt.Fprintf(errW, "unknown --framework %q (supported: testng, junit5, playwright, pytest, xunit)\n", framework)
		return 2
	}

	_ = cfg
	_ = pkg
	_ = dryRun
	_ = force
	// Orchestration is wired in Task 8.
	return 0
}

func knownKind(s string) bool {
	for _, k := range initscaffold.AllKinds {
		if string(k) == s {
			return true
		}
	}
	return false
}
