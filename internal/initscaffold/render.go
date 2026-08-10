package initscaffold

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// LogFlavor selects which per-test log appender the glue is scaffolded for.
// The rendered class name (SuluLogAppender) and its drainCurrentThread() API are
// identical across flavors, so the flush glue in the extension/listener templates
// stays flavor-agnostic — only the appender's source file differs.
type LogFlavor string

const (
	LogNone    LogFlavor = ""
	LogLog4j2  LogFlavor = "log4j2"
	LogLogback LogFlavor = "logback"
)

// logMarkers map a template path segment to the flavor it belongs to. A Java
// package segment can never contain "-", so these can't collide with a user package.
var logMarkers = map[string]LogFlavor{
	"_logs-log4j2/":  LogLog4j2,
	"_logs-logback/": LogLogback,
}

type RenderOptions struct {
	Dir     string
	Package string
	Force   bool
	DryRun  bool
	Logs    LogFlavor
}

type Action struct {
	Path string // project-relative
	Verb string // "create" | "skip" | "overwrite" | "skip (drift; --force to overwrite)"
}

// Render writes (or plans, under DryRun) the framework's glue files into opt.Dir.
func Render(fw Framework, opt RenderOptions) ([]Action, error) {
	root := "templates/" + string(fw.Kind)
	pkgPath := strings.ReplaceAll(opt.Package, ".", "/")

	var actions []Action
	err := fs.WalkDir(templatesFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, root+"/")
		// Handle the log-glue marker BEFORE package substitution: at this point the
		// package is still the literal "__PKG__", so the only marker present is ours
		// (a user package segment can't interfere — markers contain "-").
		if marker, flavor, ok := logMarker(rel); ok {
			if opt.Logs != flavor {
				return nil // log-only glue: not requested, or the other flavor
			}
			rel = strings.Replace(rel, marker, "", 1)
		}
		if fw.JavaPackage {
			rel = strings.Replace(rel, "__PKG__", pkgPath, 1)
		}
		isTmpl := strings.HasSuffix(rel, ".tmpl")
		rel = strings.TrimSuffix(rel, ".tmpl")

		raw, rerr := fs.ReadFile(templatesFS, p)
		if rerr != nil {
			return rerr
		}
		content := raw
		if isTmpl {
			tpl, perr := template.New(rel).Parse(string(raw))
			if perr != nil {
				return perr
			}
			var buf bytes.Buffer
			if eerr := tpl.Execute(&buf, struct {
				Package  string
				WithLogs bool
			}{opt.Package, opt.Logs != LogNone}); eerr != nil {
				return eerr
			}
			content = buf.Bytes()
		}
		content = withStamp(rel, content, fw.Kind)

		dest := filepath.Join(opt.Dir, rel)
		verb, write := plan(dest, content, opt.Force)
		actions = append(actions, Action{Path: rel, Verb: verb})
		if write && !opt.DryRun {
			if mkerr := os.MkdirAll(filepath.Dir(dest), 0o755); mkerr != nil {
				return mkerr
			}
			if werr := os.WriteFile(dest, content, 0o644); werr != nil {
				return werr
			}
		}
		return nil
	})
	return actions, err
}

// logMarker returns the log-glue marker segment in rel and the flavor it belongs to.
func logMarker(rel string) (marker string, flavor LogFlavor, ok bool) {
	for m, f := range logMarkers {
		if strings.Contains(rel, m) {
			return m, f, true
		}
	}
	return "", LogNone, false
}

// plan decides the verb and whether to write, comparing any existing file.
func plan(dest string, content []byte, force bool) (verb string, write bool) {
	existing, err := os.ReadFile(dest)
	if os.IsNotExist(err) {
		return "create", true
	}
	if err == nil && bytes.Equal(existing, content) {
		return "skip", false
	}
	if force {
		return "overwrite", true
	}
	return "skip (drift; --force to overwrite)", false
}

// withStamp prepends a managed-file marker where the file type supports a line comment.
func withStamp(rel string, content []byte, kind Kind) []byte {
	stamp := "suluctl-glue: v1 (managed — regenerate: suluctl init --framework " + string(kind) + " --force)"
	var prefix string
	switch {
	case strings.HasSuffix(rel, ".java"), strings.HasSuffix(rel, ".cs"):
		prefix = "// " + stamp + "\n"
	case strings.HasSuffix(rel, ".py"), strings.HasSuffix(rel, ".properties"),
		strings.Contains(rel, "META-INF/services/"):
		prefix = "# " + stamp + "\n"
	default: // .json and anything else: no comment syntax -> no stamp
		return content
	}
	return append([]byte(prefix), content...)
}
