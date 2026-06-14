package syncids

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

// pytestFramework: fullName = "tests.<module>#<func>" for top-level test functions (no classes,
// no parametrize). Assumes rootdir = suite root and testpaths = ["tests"].
type pytestFramework struct{}

var (
	pyDef  = regexp.MustCompile(`(?m)^def (test_\w+)\s*\(`) // column-0 only (module-level)
	pySulu = regexp.MustCompile(`@sulu_test\(\s*[^)]*id\s*=\s*["']([^"']*)["']`)
)

func (f *pytestFramework) TestFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func (f *pytestFramework) Parse(file string, content []byte) ([]TestRef, error) {
	src := string(content)
	module := strings.TrimSuffix(filepath.Base(file), ".py")
	var refs []TestRef
	for _, loc := range pyDef.FindAllStringSubmatchIndex(src, -1) {
		fn := src[loc[2]:loc[3]]
		hasID := pySulu.MatchString(decoratorBlock(src, loc[0]))
		refs = append(refs, TestRef{
			File: file, FullName: "tests." + module + "#" + fn, HasID: hasID, Display: fn,
		})
	}
	return refs, nil
}

// decoratorBlock returns the contiguous @decorator lines immediately above defStart.
func decoratorBlock(src string, defStart int) string {
	lines := strings.Split(src[:defStart], "\n")
	// Drop trailing empty lines (the split artifact after the final newline + any blank gap).
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	i := len(lines) - 1
	for i >= 0 && strings.HasPrefix(strings.TrimSpace(lines[i]), "@") {
		i--
	}
	return strings.Join(lines[i+1:], "\n")
}

func (f *pytestFramework) Write(content []byte, ref TestRef, displayID string) ([]byte, bool, error) {
	if ref.HasID {
		return content, false, nil
	}
	src := string(content)
	fn := ref.FullName[strings.Index(ref.FullName, "#")+1:]
	re := regexp.MustCompile(`(?m)^def ` + regexp.QuoteMeta(fn) + `\s*\(`)
	loc := re.FindStringIndex(src)
	if loc == nil {
		return content, false, fmt.Errorf("def %s not found", fn)
	}
	deco := fmt.Sprintf("@sulu_test(id=%q)\n", displayID)
	out := src[:loc[0]] + deco + src[loc[0]:]
	out = ensurePyImport(out)
	return []byte(out), true, nil
}

func ensurePyImport(src string) string {
	imp := "from sulu_pytest import sulu_test"
	if strings.Contains(src, imp) {
		return src
	}
	reImp := regexp.MustCompile(`(?m)^(?:import |from ).*$`)
	if locs := reImp.FindAllStringIndex(src, -1); len(locs) > 0 {
		at := locs[len(locs)-1][1]
		return src[:at] + "\n" + imp + src[at:]
	}
	return imp + "\n" + src
}
