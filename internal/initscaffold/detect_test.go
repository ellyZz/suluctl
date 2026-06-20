package initscaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectLog4j2(t *testing.T) {
	dir := t.TempDir()
	if DetectLog4j2(dir) {
		t.Error("empty dir must not detect log4j2")
	}
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"),
		[]byte("dependencies { testImplementation 'org.apache.logging.log4j:log4j-core:2.23.1' }"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !DetectLog4j2(dir) {
		t.Error("build.gradle with log4j-core must detect log4j2")
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  Kind
		isErr bool
	}{
		{"testng", map[string]string{"build.gradle": "testImplementation 'io.qameta.allure:allure-testng:2.34.0'\nuseTestNG()\norg.testng"}, TestNG, false},
		{"junit5", map[string]string{"build.gradle": "testImplementation 'org.junit.jupiter:junit-jupiter:5.10.2'"}, JUnit5, false},
		{"playwright", map[string]string{"package.json": `{"devDependencies":{"@playwright/test":"^1.53.0"}}`}, Playwright, false},
		{"pytest", map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\naddopts='-v'"}, Pytest, false},
		{"xunit", map[string]string{"Api.csproj": `<PackageReference Include="xunit.v3" Version="3.0.0" />`}, XUnit, false},
		{"none", map[string]string{"README.md": "hi"}, "", true},
		{"ambiguous", map[string]string{"build.gradle": "org.testng\norg.junit.jupiter:junit-jupiter"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for n, b := range tc.files {
				write(t, dir, n, b)
			}
			got, err := Detect(dir)
			if tc.isErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
