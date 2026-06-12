package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScannerStabilityAndSentTracking(t *testing.T) {
	dir := t.TempDir()
	s := NewScanner(dir)

	// scan 1: empty
	if files, err := s.Scan(); err != nil || len(files) != 0 {
		t.Fatalf("empty: %v %v", files, err)
	}

	p := filepath.Join(dir, "a-result.json")
	writeFile(t, p, `{"uuid":"a"}`)

	// scan 2: file just appeared — no previous snapshot entry, NOT ready
	if files, _ := s.Scan(); len(files) != 0 {
		t.Fatalf("fresh file must wait one tick, got %v", names(files))
	}
	// scan 3: stable across two scans — ready
	files, _ := s.Scan()
	if len(files) != 1 || files[0].Path != p {
		t.Fatalf("want ready, got %v", names(files))
	}
	s.MarkSent(files)

	// scan 4: sent and unchanged — not returned
	if files, _ := s.Scan(); len(files) != 0 {
		t.Fatalf("sent file returned again: %v", names(files))
	}

	// modify the file (bump mtime explicitly — coarse-mtime filesystems)
	writeFile(t, p, `{"uuid":"a","status":"passed"}`)
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if files, _ := s.Scan(); len(files) != 0 {
		t.Fatalf("changed file must re-stabilize first: %v", names(files))
	}
	files, _ = s.Scan()
	if len(files) != 1 {
		t.Fatalf("changed file must be re-sent: %v", names(files))
	}
}

func TestScannerMissingDirIsEmpty(t *testing.T) {
	s := NewScanner(filepath.Join(t.TempDir(), "not-created-yet"))
	if files, err := s.Scan(); err != nil || len(files) != 0 {
		t.Fatalf("missing dir must scan as empty: %v %v", files, err)
	}
}

func TestSweepAllIgnoresStability(t *testing.T) {
	dir := t.TempDir()
	s := NewScanner(dir)
	writeFile(t, filepath.Join(dir, "late-result.json"), "{}")
	files, err := s.SweepAll()
	if err != nil || len(files) != 1 {
		t.Fatalf("sweep must return fresh files immediately: %v %v", files, err)
	}
	s.MarkSent(files)
	if files, _ := s.SweepAll(); len(files) != 0 {
		t.Fatalf("sweep must skip sent files: %v", names(files))
	}
}
