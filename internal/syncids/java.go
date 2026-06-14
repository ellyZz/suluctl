package syncids

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// javaFramework handles testng + junit5 (identical @SuluTest mechanics). The Allure fullName is
// "<fully.qualified.ClassName>.<methodName>" (dot) for non-nested, non-parameterized tests.
type javaFramework struct{ pkg string }

var (
	javaPkg    = regexp.MustCompile(`(?m)^\s*package\s+([a-zA-Z_][\w.]*)\s*;`)
	javaClass  = regexp.MustCompile(`(?m)^\s*(?:public\s+)?(?:final\s+|abstract\s+)?class\s+(\w+)`)
	javaTest   = regexp.MustCompile(`@Test\b`)
	javaMethod = regexp.MustCompile(`(?m)^\s*(?:public|protected|private)\s+[\w<>\[\], ]+\s+(\w+)\s*\(`)
	javaSulu   = regexp.MustCompile(`@SuluTest\s*\(([^)]*)\)`)
	javaSuluID = regexp.MustCompile(`\bid\s*=\s*"([^"]*)"`)
)

func (f *javaFramework) TestFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".java") {
			return nil
		}
		if strings.HasSuffix(path, "SuluTest.java") { // skip the glue annotation itself
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func (f *javaFramework) Parse(file string, content []byte) ([]TestRef, error) {
	src := string(content)
	pkg := f.pkg
	if m := javaPkg.FindStringSubmatch(src); m != nil {
		pkg = m[1]
	}
	cm := javaClass.FindStringSubmatch(src)
	if cm == nil {
		return nil, nil
	}
	class := cm[1]
	fqcn := class
	if pkg != "" {
		fqcn = pkg + "." + class
	}

	var refs []TestRef
	for _, loc := range javaTest.FindAllStringIndex(src, -1) {
		tail := src[loc[1]:]
		mLoc := javaMethod.FindStringSubmatchIndex(tail)
		if mLoc == nil {
			continue
		}
		method := tail[mLoc[2]:mLoc[3]]
		// Window from a little before @Test up to the method declaration line — spans the whole
		// annotation block so an existing @SuluTest(id=…) (which sits between @Test and the method)
		// is detected. (Must not cut at the first "(" — @Test(...) / @SuluTest(...) have parens too.)
		windowStart := loc[0] - 200
		if windowStart < 0 {
			windowStart = 0
		}
		window := src[windowStart : loc[1]+mLoc[0]]
		hasID := false
		if sm := javaSulu.FindStringSubmatch(window); sm != nil {
			if im := javaSuluID.FindStringSubmatch(sm[1]); im != nil && im[1] != "" {
				hasID = true
			}
		}
		refs = append(refs, TestRef{
			File: file, FullName: fqcn + "." + method, HasID: hasID, Display: class + "." + method,
		})
	}
	return refs, nil
}

func (f *javaFramework) Write(content []byte, ref TestRef, displayID string) ([]byte, bool, error) {
	if ref.HasID {
		return content, false, nil
	}
	src := string(content)
	method := ref.FullName[strings.LastIndex(ref.FullName, ".")+1:]
	mre := regexp.MustCompile(`(?m)^(\s*)((?:public|protected|private)\s+[\w<>\[\], ]+\s+` +
		regexp.QuoteMeta(method) + `\s*\()`)
	loc := mre.FindStringSubmatchIndex(src)
	if loc == nil {
		return content, false, fmt.Errorf("method %s not found for rewrite", method)
	}
	indent := src[loc[2]:loc[3]]
	// Anchor insertion directly above the @Test line if present, else above the method.
	before := src[:loc[0]]
	insertAt := loc[0]
	if testIdx := strings.LastIndex(before, "@Test"); testIdx >= 0 {
		insertAt = strings.LastIndex(before[:testIdx], "\n") + 1
	}
	annotation := indent + fmt.Sprintf("@SuluTest(id = %q)\n", displayID)
	out := src[:insertAt] + annotation + src[insertAt:]
	out = ensureJavaImport(out, f.importFQN())
	return []byte(out), true, nil
}

func (f *javaFramework) importFQN() string {
	pkg := f.pkg
	if pkg == "" {
		pkg = "sulu"
	}
	return pkg + ".annotation.SuluTest"
}

func ensureJavaImport(src, fqn string) string {
	imp := "import " + fqn + ";"
	if strings.Contains(src, imp) {
		return src
	}
	if idx := strings.LastIndex(src, "\nimport "); idx >= 0 {
		eol := strings.Index(src[idx+1:], "\n")
		at := idx + 1 + eol + 1
		return src[:at] + imp + "\n" + src[at:]
	}
	if pm := javaPkg.FindStringIndex(src); pm != nil {
		eol := strings.Index(src[pm[1]:], "\n")
		at := pm[1] + eol + 1
		return src[:at] + "\n" + imp + "\n" + src[at:]
	}
	return imp + "\n" + src
}

// ResolveJavaImportPackage finds the project's SuluTest.java and returns its declared package, so the
// import FQN matches wherever Layer 1 `init --package` placed the annotation. Empty if not found.
func ResolveJavaImportPackage(root string) string {
	found := ""
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "SuluTest.java") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if m := javaPkg.FindSubmatch(data); m != nil {
			found = string(m[1]) // e.g. "ai.sulu.common.annotation" → strip the trailing ".annotation"
			found = strings.TrimSuffix(found, ".annotation")
		}
		return nil
	})
	return found
}
