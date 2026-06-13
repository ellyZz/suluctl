package initscaffold

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ellyZz/suluctl/internal/config"
)

// DetectCI returns "github", "gitlab", or "" based on repo CI config presence.
func DetectCI(dir string) string {
	if entries, err := os.ReadDir(filepath.Join(dir, ".github", "workflows")); err == nil && len(entries) > 0 {
		return "github"
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitlab-ci.yml")); err == nil {
		return "gitlab"
	}
	return ""
}

// WatchCommand renders a ready-to-run suluctl watch line for the framework.
func WatchCommand(fw Framework, cfg config.Config) string {
	name := cfg.LaunchName
	if name == "" {
		name = "my test run"
	}
	return fmt.Sprintf("suluctl watch --results %s --launch-name %q -- %s", fw.ResultsDir, name, fw.TestCmd)
}

func ciSnippet(kind string, fw Framework) string {
	watch := fmt.Sprintf("suluctl watch --results %s --launch-name \"ci run\" -- %s", fw.ResultsDir, fw.TestCmd)
	switch kind {
	case "github":
		return "GitHub Actions step:\n" +
			"  - name: Tests -> Sulu\n" +
			"    env: { SULU_URL: ${{ secrets.SULU_URL }}, SULU_TOKEN: ${{ secrets.SULU_TOKEN }}, SULU_PROJECT_ID: ${{ vars.SULU_PROJECT_ID }} }\n" +
			"    run: " + watch
	case "gitlab":
		return "GitLab CI job:\n" +
			"  tests:\n    script:\n      - " + watch + "\n    # set SULU_URL / SULU_TOKEN / SULU_PROJECT_ID as CI variables"
	default:
		return "CI step (any platform): set SULU_URL, SULU_TOKEN, SULU_PROJECT_ID, then run:\n  " + watch
	}
}

// PrintReport writes the summary, manual checklist, ready watch command, and CI snippet.
func PrintReport(w io.Writer, fw Framework, actions []Action, patch PatchResult, dir string, cfg config.Config) {
	fmt.Fprintf(w, "Sulu glue for %s\n\n", fw.Display)

	fmt.Fprintln(w, "Files:")
	for _, a := range actions {
		fmt.Fprintf(w, "  %-30s %s\n", a.Verb, a.Path)
	}

	fmt.Fprintln(w, "\nBuild manifest:")
	if patch.Changed {
		fmt.Fprintf(w, "  patched %s\n", patch.Path)
	} else if patch.Printed != "" {
		fmt.Fprintf(w, "  %s\n", patch.Printed)
	} else {
		fmt.Fprintf(w, "  already configured (%s)\n", patch.Path)
	}

	if len(fw.ManualSteps) > 0 {
		fmt.Fprintln(w, "\nManual steps:")
		for _, s := range fw.ManualSteps {
			fmt.Fprintf(w, "  - %s\n", s)
		}
	}

	fmt.Fprintln(w, "\nRun your tests through Sulu:")
	fmt.Fprintf(w, "  %s\n", WatchCommand(fw, cfg))

	fmt.Fprintln(w, "\n"+ciSnippet(DetectCI(dir), fw))
}
