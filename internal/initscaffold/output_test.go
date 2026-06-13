package initscaffold

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ellyZz/suluctl/internal/config"
)

func TestDetectCI(t *testing.T) {
	gh := t.TempDir()
	write(t, gh, ".github/workflows/ci.yml", "on: push")
	if DetectCI(gh) != "github" {
		t.Errorf("github not detected")
	}
	gl := t.TempDir()
	write(t, gl, ".gitlab-ci.yml", "stages: [test]")
	if DetectCI(gl) != "gitlab" {
		t.Errorf("gitlab not detected")
	}
	if DetectCI(t.TempDir()) != "" {
		t.Errorf("empty repo should detect no CI")
	}
}

func TestWatchCommandUsesResultsDirAndTestCmd(t *testing.T) {
	cmd := WatchCommand(Registry(TestNG), config.Config{LaunchName: "ci run"})
	if !strings.Contains(cmd, "--results build/allure-results") {
		t.Errorf("watch cmd missing results dir: %s", cmd)
	}
	if !strings.Contains(cmd, "-- ./gradlew test") {
		t.Errorf("watch cmd missing test command: %s", cmd)
	}
}

func TestPrintReportShowsManualStepsAndSnippet(t *testing.T) {
	var b bytes.Buffer
	fw := Registry(XUnit)
	PrintReport(&b, fw, []Action{{Path: "Support/SuluTestAttribute.cs", Verb: "create"}},
		PatchResult{Path: "Api.csproj", Changed: true}, t.TempDir(), config.Config{})
	out := b.String()
	if !strings.Contains(out, "SuluTest") {
		t.Errorf("report missing xUnit manual caveat:\n%s", out)
	}
	if !strings.Contains(out, "suluctl watch") {
		t.Errorf("report missing watch command:\n%s", out)
	}
}
