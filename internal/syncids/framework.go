// Package syncids implements `suluctl sync-ids`: scan test source, compute each test's structural
// Allure fullName, resolve it to the pretty displayId via the backend, and write the framework's
// sulu_id token back into source. Regex over source (stdlib-only, no AST), mirroring initscaffold.
package syncids

import "github.com/ellyZz/suluctl/internal/initscaffold"

// TestRef is one discovered test with the data needed to resolve + rewrite it.
type TestRef struct {
	File     string // absolute path
	FullName string // Allure fullName == the resolve key (the importer's stored testId)
	HasID    bool   // a non-empty sulu_id token already present (skip unless --force)
	EmptyID  bool   // an empty placeholder present (xunit [SuluTest("")]) → fillable
	Display  string // human label for the report (e.g. "ClassName.method")
}

// Options carries adapter inputs that vary per project.
type Options struct {
	JavaPackage string // Java base package fallback (used for the import FQN when no SuluTest.java found)
	TestDir     string // playwright testDir (relative-path root for fullName)
}

// Framework parses source files and rewrites a single test's sulu_id token.
type Framework interface {
	// TestFiles returns candidate source files under root (absolute paths).
	TestFiles(root string) ([]string, error)
	// Parse extracts every test in a file's content.
	Parse(file string, content []byte) ([]TestRef, error)
	// Write inserts/fills the sulu_id token for ref with displayID, ensuring any import.
	// Returns the new content and whether it changed.
	Write(content []byte, ref TestRef, displayID string) ([]byte, bool, error)
}

// For returns the adapter for a detected framework kind, or nil if unsupported.
func For(kind initscaffold.Kind, opts Options) Framework {
	switch string(kind) {
	case "testng", "junit5":
		return &javaFramework{pkg: opts.JavaPackage}
	case "pytest":
		return &pytestFramework{}
	case "playwright":
		return &playwrightFramework{testDir: opts.TestDir}
	case "xunit":
		return &xunitFramework{}
	}
	return nil
}
