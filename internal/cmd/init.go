package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

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

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(errW, err)
		return 1
	}
	var kind initscaffold.Kind
	if framework != "" {
		kind = initscaffold.Kind(framework)
	} else {
		k, derr := initscaffold.Detect(dir)
		if derr != nil {
			fmt.Fprintln(errW, derr)
			return 1
		}
		kind = k
	}
	fw := initscaffold.Registry(kind)

	if fw.JavaPackage && pkg == "" {
		pkg = initscaffold.DetectJavaBasePackage(dir)
	}

	withLogs := fw.JavaPackage && initscaffold.DetectLog4j2(dir)

	actions, err := initscaffold.Render(fw, initscaffold.RenderOptions{
		Dir: dir, Package: pkg, Force: force, DryRun: dryRun, WithLogs: withLogs,
	})
	if err != nil {
		fmt.Fprintf(errW, "scaffold failed: %v\n", err)
		return 1
	}
	patch, err := initscaffold.PatchManifest(fw, dir, dryRun)
	if err != nil {
		fmt.Fprintf(errW, "manifest patch failed: %v\n", err)
		return 1
	}
	if withLogs {
		fw.ManualSteps = append(fw.ManualSteps, initscaffold.Log4j2SetupSteps(pkg)...)
	} else if fw.JavaPackage {
		fw.ManualSteps = append(fw.ManualSteps, initscaffold.Log4j2HintSteps()...)
	}
	if dryRun {
		fmt.Fprintln(out, "DRY RUN — no files written")
	}
	initscaffold.PrintReport(out, fw, actions, patch, dir, cfg)
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
