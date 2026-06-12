package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(files []FileState) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = filepath.Base(f.Path)
	}
	return out
}

func TestCollectDirRecursiveSkipsHidden(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a-result.json"), "{}")
	writeFile(t, filepath.Join(dir, "history", "history.json"), "{}")
	writeFile(t, filepath.Join(dir, ".hidden"), "x")
	writeFile(t, filepath.Join(dir, ".git", "config"), "x")
	files, err := Collect(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := names(files)
	if len(got) != 2 || got[0] != "a-result.json" || got[1] != "history.json" {
		t.Errorf("got %v", got)
	}
	if files[0].Size != 2 {
		t.Errorf("size not populated: %+v", files[0])
	}
}

func TestCollectSingleFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "junit.xml")
	writeFile(t, p, "<testsuite/>")
	files, err := Collect(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != p {
		t.Errorf("got %+v", files)
	}
}

func TestCollectGlob(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "TEST-a.xml"), "<x/>")
	writeFile(t, filepath.Join(dir, "TEST-b.xml"), "<x/>")
	writeFile(t, filepath.Join(dir, "notes.txt"), "x")
	files, err := Collect(filepath.Join(dir, "TEST-*.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("got %v", names(files))
	}
}

func TestCollectEmptyIsError(t *testing.T) {
	if _, err := Collect(t.TempDir()); err == nil {
		t.Error("empty dir must be an error")
	}
	if _, err := Collect("/nonexistent/nothing-*"); err == nil {
		t.Error("no glob matches must be an error")
	}
}
