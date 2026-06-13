package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitHelpExitsZero(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	if code := Init([]string{"-h"}, &out, &errB, "test"); code != 0 {
		t.Fatalf("help exit = %d, want 0; stderr=%s", code, errB.String())
	}
}

func TestInitUnknownFrameworkIsUsageError(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	code := Init([]string{"--framework", "nope", "--dry-run"}, &out, &errB, "test")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr=%s", code, errB.String())
	}
}

func TestInitPlaywrightEndToEnd(t *testing.T) {
	neutralizeEnv(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"),
		[]byte("{\n  \"devDependencies\": {\n    \"@playwright/test\": \"^1.53.0\"\n  }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out, errB bytes.Buffer
	code := Init([]string{}, &out, &errB, "test") // autodetect playwright
	if code != 0 {
		t.Fatalf("exit %d; stderr=%s", code, errB.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "tests/support/sulu.ts")); err != nil {
		t.Errorf("sulu.ts not written: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	if !strings.Contains(string(data), "allure-playwright") {
		t.Errorf("package.json not patched:\n%s", data)
	}
	if !strings.Contains(out.String(), "suluctl watch --results allure-results") {
		t.Errorf("missing watch command:\n%s", out.String())
	}
}

func TestInitDryRunWritesNothing(t *testing.T) {
	neutralizeEnv(t)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.pytest.ini_options]\n"), 0o644)
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(dir)

	var out, errB bytes.Buffer
	if code := Init([]string{"--dry-run"}, &out, &errB, "test"); code != 0 {
		t.Fatalf("exit %d; stderr=%s", code, errB.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "sulu_pytest.py")); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote sulu_pytest.py")
	}
}
