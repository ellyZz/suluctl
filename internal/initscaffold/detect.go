package initscaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Detect inspects dir and returns the single matching framework.
// 0 matches or >1 matches is an error whose message guides the user to --framework.
func Detect(dir string) (Kind, error) {
	gradleMaven := readIfAny(dir, "build.gradle", "build.gradle.kts", "pom.xml")
	pyConfig := readIfAny(dir, "pyproject.toml", "pytest.ini", "setup.cfg")
	pkgJSON := readIfAny(dir, "package.json")
	csproj := readGlob(dir, "*.csproj")

	var found []Kind
	if strings.Contains(gradleMaven, "org.testng") {
		found = append(found, TestNG)
	}
	// match either the artifact id (junit-jupiter) or the package root (org.junit.jupiter)
	if strings.Contains(gradleMaven, "junit-jupiter") || strings.Contains(gradleMaven, "org.junit.jupiter") {
		found = append(found, JUnit5)
	}
	if strings.Contains(pkgJSON, "@playwright/test") {
		found = append(found, Playwright)
	}
	if strings.Contains(pyConfig, "pytest") {
		found = append(found, Pytest)
	}
	if strings.Contains(csproj, "xunit") {
		found = append(found, XUnit)
	}

	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("no supported test framework detected in %s\nsupported: testng, junit5, playwright, pytest, xunit\npass --framework X to choose explicitly", dir)
	default:
		names := make([]string, len(found))
		for i, k := range found {
			names[i] = string(k)
		}
		sort.Strings(names)
		return "", fmt.Errorf("multiple frameworks detected (%s); pass --framework X to choose", strings.Join(names, ", "))
	}
}

// readIfAny returns the concatenated contents of the first existing names.
func readIfAny(dir string, names ...string) string {
	var b strings.Builder
	for _, n := range names {
		if data, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
			b.Write(data)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// readGlob returns the concatenated contents of files matching pattern in dir (non-recursive).
func readGlob(dir, pattern string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	var b strings.Builder
	for _, m := range matches {
		if data, err := os.ReadFile(m); err == nil {
			b.Write(data)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
