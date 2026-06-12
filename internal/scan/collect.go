package scan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	sortByPath(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("no files found at %q", results)
	}
	return out, nil
}

func collect(results string) ([]FileState, error) {
	info, statErr := os.Stat(results)
	if statErr == nil {
		if info.IsDir() {
			// os.Stat follows symlinks, but WalkDir Lstats its root and won't
			// descend into a symlinked directory. Resolve first.
			if resolved, rerr := filepath.EvalSymlinks(results); rerr == nil {
				results = resolved
			}
			return walkDir(results)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("--results %q is not a regular file", results)
		}
		return []FileState{{Path: results, Size: info.Size(), ModTime: info.ModTime()}}, nil
	}
	// Only treat as a glob pattern when the path actually contains meta characters.
	// A meta-character-free path that failed stat (e.g. a dangling symlink) is
	// simply an error — recursing via Glob would cause infinite recursion because
	// Glob (Lstat-based) would return the path itself as a match.
	if !strings.ContainsAny(results, "*?[\\") {
		return nil, fmt.Errorf("cannot read --results %q: %w", results, statErr)
	}
	matches, err := filepath.Glob(results)
	if err != nil {
		return nil, fmt.Errorf("bad --results pattern %q: %w", results, err)
	}
	var out []FileState
	for _, m := range matches {
		if m == results {
			continue // guard against a Glob returning the pattern itself
		}
		// Skip glob matches whose stat fails (e.g. dangling symlinks).
		if _, serr := os.Stat(m); serr != nil {
			continue
		}
		sub, err := collect(m)
		if err != nil {
			return nil, err
		}
		out = append(out, sub...)
	}
	return out, nil
}

// walkDir returns all regular files under dir, skipping symlinks, dot-files,
// dot-dirs, and other irregular files.
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
