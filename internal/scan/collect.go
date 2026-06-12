package scan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileState identifies a file's content state at observation time.
type FileState struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// sameAs compares by size + mtime (mtime via Equal — never == on time.Time).
func (f FileState) sameAs(o FileState) bool {
	return f.Size == o.Size && f.ModTime.Equal(o.ModTime)
}

// Collect resolves --results for the upload command: a directory (recursive),
// a single file, or a glob pattern whose matches are collected recursively.
func Collect(results string) ([]FileState, error) {
	out, err := collect(results)
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if len(out) == 0 {
		return nil, fmt.Errorf("no files found at %q", results)
	}
	return out, nil
}

func collect(results string) ([]FileState, error) {
	if info, err := os.Stat(results); err == nil {
		if info.IsDir() {
			return walkDir(results)
		}
		return []FileState{{Path: results, Size: info.Size(), ModTime: info.ModTime()}}, nil
	}
	matches, err := filepath.Glob(results)
	if err != nil {
		return nil, fmt.Errorf("bad --results pattern %q: %w", results, err)
	}
	var out []FileState
	for _, m := range matches {
		sub, err := collect(m)
		if err != nil {
			return nil, err
		}
		out = append(out, sub...)
	}
	return out, nil
}

// walkDir returns all regular files under dir, skipping dot-files and dot-dirs.
func walkDir(dir string) ([]FileState, error) {
	var out []FileState
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // file vanished mid-walk — skip
		}
		if strings.HasPrefix(d.Name(), ".") && path != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, FileState{Path: path, Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	return out, err
}
