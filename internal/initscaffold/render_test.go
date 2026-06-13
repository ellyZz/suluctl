package initscaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTestNGSubstitutesPackageAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	fw := Registry(TestNG)
	opt := RenderOptions{Dir: dir, Package: "com.acme.qa"}

	actions, err := Render(fw, opt)
	if err != nil {
		t.Fatal(err)
	}
	listener := filepath.Join(dir, "src/test/java/com/acme/qa/SuluLabelListener.java")
	data, err := os.ReadFile(listener)
	if err != nil {
		t.Fatalf("listener not written: %v", err)
	}
	if !strings.Contains(string(data), "package com.acme.qa;") {
		t.Errorf("package not substituted:\n%s", data)
	}
	if !strings.Contains(string(data), "suluctl-glue: v1") {
		t.Errorf("version stamp missing")
	}
	spi, _ := os.ReadFile(filepath.Join(dir, "src/test/resources/META-INF/services/org.testng.ITestNGListener"))
	if !strings.Contains(string(spi), "com.acme.qa.SuluLabelListener") {
		t.Errorf("SPI class not substituted:\n%s", spi)
	}
	if !hasVerb(actions, listener, "create") {
		t.Errorf("expected create action for listener; got %+v", actions)
	}

	actions2, err := Render(fw, opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range actions2 {
		if a.Verb == "create" || a.Verb == "overwrite" {
			t.Errorf("re-render mutated %s (verb %q)", a.Path, a.Verb)
		}
	}
}

func TestRenderDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(Registry(Pytest), RenderOptions{Dir: dir, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sulu_pytest.py")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote a file")
	}
}

func hasVerb(actions []Action, absPath, verb string) bool {
	for _, a := range actions {
		if strings.HasSuffix(absPath, a.Path) && a.Verb == verb {
			return true
		}
	}
	return false
}
