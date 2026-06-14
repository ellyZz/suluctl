package syncids

import (
	"fmt"
	"os"

	"github.com/ellyZz/suluctl/internal/api"
	"github.com/ellyZz/suluctl/internal/initscaffold"
)

// Config is the input to Run.
type Config struct {
	Client    *api.Client
	ProjectID int64
	Kind      initscaffold.Kind
	Dir       string
	Opts      Options
	DryRun    bool
	Force     bool
}

// Result tallies the outcome.
type Result struct {
	Resolved   int
	Written    int
	AlreadySet int
	NotFound   int
	Errors     int
}

// Run scans Dir, resolves each test's fullName, and writes the displayId back into source.
// Each adapter's Write re-anchors by name (Java/pytest/xunit) or inserts inline (playwright), so
// applying writes top-to-bottom on the progressively-modified content is position-safe.
func Run(cfg Config) (Result, error) {
	fw := For(cfg.Kind, cfg.Opts)
	if fw == nil {
		return Result{}, fmt.Errorf("unsupported framework %q", cfg.Kind)
	}
	files, err := fw.TestFiles(cfg.Dir)
	if err != nil {
		return Result{}, err
	}
	var res Result
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			res.Errors++
			continue
		}
		refs, err := fw.Parse(file, content)
		if err != nil {
			res.Errors++
			continue
		}
		changed := false
		for _, ref := range refs {
			if ref.HasID && !cfg.Force {
				res.AlreadySet++
				continue
			}
			resolved, found, rerr := cfg.Client.ResolveTestCase(cfg.ProjectID, ref.FullName)
			if rerr != nil {
				res.Errors++
				continue
			}
			if !found {
				res.NotFound++
				continue
			}
			res.Resolved++
			next, didChange, werr := fw.Write(content, ref, resolved.DisplayID)
			if werr != nil {
				res.Errors++
				continue
			}
			if didChange {
				content = next
				changed = true
				res.Written++
			} else {
				res.AlreadySet++
			}
		}
		if changed && !cfg.DryRun {
			if err := os.WriteFile(file, content, 0o644); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}
