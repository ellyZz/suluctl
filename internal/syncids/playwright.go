package syncids

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// playwrightFramework: fullName = "<fileRelToTestDir>:<line>:<col>" (allure-playwright). The annotation
// is inserted INLINE as the test() 2nd arg, so it never shifts line numbers. NOTE: the key is
// line-number-dependent — sync-ids should run right after watch, before further edits (documented).
type playwrightFramework struct {
	testDir string // path root for fullName (relative to repo root); default "tests"
}

// Matches a test() call; captures leading indent (for column) and the title. Uses [ \t]* not \s*
// because Go's \s includes newlines, which would let the match span preceding blank lines.
var pwTest = regexp.MustCompile("(?m)^([ \\t]*)test\\(\\s*['\"`]([^'\"`]*)['\"`]")

func (f *playwrightFramework) TestFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".spec.ts") || strings.HasSuffix(path, ".spec.js") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func (f *playwrightFramework) Parse(file string, content []byte) ([]TestRef, error) {
	src := string(content)
	rel := f.relToTestDir(file)
	var refs []TestRef
	for _, loc := range pwTest.FindAllStringSubmatchIndex(src, -1) {
		indent := src[loc[2]:loc[3]]
		title := src[loc[4]:loc[5]]
		line := 1 + strings.Count(src[:loc[0]], "\n")
		col := len(indent) + 1
		tail := src[loc[1]:min(loc[1]+200, len(src))]
		hasID := strings.Contains(tail, "type: 'sulu'") || strings.Contains(tail, `type: "sulu"`)
		refs = append(refs, TestRef{
			File: file, FullName: fmt.Sprintf("%s:%d:%d", rel, line, col), HasID: hasID, Display: title,
		})
	}
	return refs, nil
}

// relToTestDir returns the file path relative to the configured testDir, posix-slashed.
func (f *playwrightFramework) relToTestDir(file string) string {
	dir := f.testDir
	if dir == "" {
		dir = "tests"
	}
	norm := filepath.ToSlash(file)
	marker := "/" + strings.Trim(filepath.ToSlash(dir), "./") + "/"
	if i := strings.LastIndex(norm, marker); i >= 0 {
		return norm[i+len(marker):]
	}
	return filepath.Base(file)
}

func (f *playwrightFramework) Write(content []byte, ref TestRef, displayID string) ([]byte, bool, error) {
	if ref.HasID {
		return content, false, nil
	}
	src := string(content)
	parts := strings.Split(ref.FullName, ":")
	if len(parts) < 3 {
		return content, false, fmt.Errorf("bad fullName %s", ref.FullName)
	}
	wantLine := parts[len(parts)-2]
	for _, loc := range pwTest.FindAllStringSubmatchIndex(src, -1) {
		line := 1 + strings.Count(src[:loc[0]], "\n")
		if fmt.Sprintf("%d", line) != wantLine {
			continue
		}
		insertAt := loc[5] + 1 // just past the closing title quote
		ann := fmt.Sprintf(", { annotation: { type: 'sulu', description: '%s' } }", displayID)
		out := src[:insertAt] + ann + src[insertAt:]
		out = ensurePwImport(out)
		return []byte(out), true, nil
	}
	return content, false, fmt.Errorf("test on line %s not found", wantLine)
}

func ensurePwImport(src string) string {
	if strings.Contains(src, "from './support/sulu'") || strings.Contains(src, `from "./support/sulu"`) {
		return src
	}
	re := regexp.MustCompile(`from\s+['"]@playwright/test['"]`)
	if re.MatchString(src) {
		return re.ReplaceAllString(src, "from './support/sulu'")
	}
	return src
}

// DetectPlaywrightTestDir reads `testDir` from playwright.config.(ts|js); default "tests".
func DetectPlaywrightTestDir(root string) string {
	re := regexp.MustCompile("testDir\\s*:\\s*['\"`]([^'\"`]+)['\"`]")
	for _, name := range []string{"playwright.config.ts", "playwright.config.js"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		if m := re.FindSubmatch(data); m != nil {
			return strings.Trim(string(m[1]), "./")
		}
	}
	return "tests"
}
