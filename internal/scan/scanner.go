package scan

import (
	"os"
	"sort"
)

// Scanner watches a directory by polling. A file is upload-ready when it is
// new-or-changed versus the sent registry AND identical between two consecutive
// scans (stability = debounce + partial-write guard in one mechanism).
type Scanner struct {
	dir  string
	prev map[string]FileState // previous scan snapshot
	sent map[string]FileState // file state at the moment it was handed out
}

func NewScanner(dir string) *Scanner {
	return &Scanner{dir: dir, prev: map[string]FileState{}, sent: map[string]FileState{}}
}

// Scan returns stable, unsent-or-changed files. A missing directory is empty, not an error.
func (s *Scanner) Scan() ([]FileState, error) {
	current, err := s.snapshot()
	if err != nil {
		return nil, err
	}
	var ready []FileState
	for path, st := range current {
		if sent, ok := s.sent[path]; ok && sent.sameAs(st) {
			continue
		}
		if prev, ok := s.prev[path]; ok && prev.sameAs(st) {
			ready = append(ready, st)
		}
	}
	s.prev = current
	sortByPath(ready)
	return ready, nil
}

// SweepAll returns everything unsent-or-changed, ignoring stability.
// Called after the test command exited — there are no more writers.
func (s *Scanner) SweepAll() ([]FileState, error) {
	current, err := s.snapshot()
	if err != nil {
		return nil, err
	}
	var ready []FileState
	for path, st := range current {
		if sent, ok := s.sent[path]; ok && sent.sameAs(st) {
			continue
		}
		ready = append(ready, st)
	}
	s.prev = current
	sortByPath(ready)
	return ready, nil
}

// MarkSent records the state files had when selected for upload. If a file
// changes afterwards, the next Scan sees state != sent and re-sends it.
func (s *Scanner) MarkSent(files []FileState) {
	for _, f := range files {
		s.sent[f.Path] = f
	}
}

func (s *Scanner) snapshot() (map[string]FileState, error) {
	if _, err := os.Stat(s.dir); err != nil {
		if os.IsNotExist(err) {
			return map[string]FileState{}, nil
		}
		return nil, err
	}
	files, err := walkDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]FileState, len(files))
	for _, f := range files {
		out[f.Path] = f
	}
	return out, nil
}

func sortByPath(files []FileState) {
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
}
