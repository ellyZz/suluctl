package initscaffold

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var pkgLine = regexp.MustCompile(`(?m)^\s*package\s+([a-zA-Z_][\w.]*)\s*;`)

// DetectJavaBasePackage returns the shallowest package declared in any test *.java
// file under dir, or "sulu" when none is found.
func DetectJavaBasePackage(dir string) string {
	best := ""
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".java") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		m := pkgLine.FindSubmatch(data)
		if m == nil {
			return nil
		}
		pkg := string(m[1])
		if best == "" || strings.Count(pkg, ".") < strings.Count(best, ".") {
			best = pkg
		}
		return nil
	})
	if best == "" {
		return "sulu"
	}
	return best
}
