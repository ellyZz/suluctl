package syncids

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

// xunitFramework: fullName = "<Namespace>.<Class>.<Method>". A [Fact] without [SuluTest] emits NO
// allure result, so there is no auto-created case to resolve — sync-ids only FILLS empty
// [SuluTest("")] placeholders and leaves non-empty (user-chosen) ids untouched.
type xunitFramework struct{}

var (
	csNamespace = regexp.MustCompile(`(?m)^\s*namespace\s+([\w.]+)`)
	csClass     = regexp.MustCompile(`(?m)^\s*(?:public\s+)?(?:sealed\s+|abstract\s+|partial\s+)*class\s+(\w+)`)
	csFactSulu  = regexp.MustCompile(`\[\s*(?:Fact|Theory)\s*,\s*SuluTest\(\s*"([^"]*)"\s*\)\s*\]`)
	csMethod    = regexp.MustCompile(`(?m)public\s+(?:async\s+)?[\w<>\[\].]+\s+(\w+)\s*\(`)
	csEmptySulu = regexp.MustCompile(`SuluTest\(\s*""\s*\)`)
)

func (f *xunitFramework) TestFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".cs") {
			return nil
		}
		if strings.HasSuffix(path, "SuluTestAttribute.cs") || strings.HasSuffix(path, "AssemblyInfo.cs") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func (f *xunitFramework) Parse(file string, content []byte) ([]TestRef, error) {
	src := string(content)
	ns := ""
	if m := csNamespace.FindStringSubmatch(src); m != nil {
		ns = m[1]
	}
	cm := csClass.FindStringSubmatch(src)
	if cm == nil {
		return nil, nil
	}
	class := cm[1]
	fqcn := class
	if ns != "" {
		fqcn = ns + "." + class
	}

	var refs []TestRef
	for _, loc := range csFactSulu.FindAllStringSubmatchIndex(src, -1) {
		id := src[loc[2]:loc[3]]
		mm := csMethod.FindStringSubmatch(src[loc[1]:])
		if mm == nil {
			continue
		}
		method := mm[1]
		refs = append(refs, TestRef{
			File: file, FullName: fqcn + "." + method,
			HasID: id != "", EmptyID: id == "", Display: class + "." + method,
		})
	}
	return refs, nil
}

func (f *xunitFramework) Write(content []byte, ref TestRef, displayID string) ([]byte, bool, error) {
	if ref.HasID || !ref.EmptyID {
		return content, false, nil // only empty placeholders are filled
	}
	src := string(content)
	method := ref.FullName[strings.LastIndex(ref.FullName, ".")+1:]
	for _, loc := range csEmptySulu.FindAllStringIndex(src, -1) {
		mm := csMethod.FindStringSubmatch(src[loc[1]:])
		if mm == nil || mm[1] != method {
			continue
		}
		out := src[:loc[0]] + fmt.Sprintf("SuluTest(%q)", displayID) + src[loc[1]:]
		return []byte(out), true, nil
	}
	return content, false, fmt.Errorf("empty SuluTest placeholder for %s not found", method)
}
