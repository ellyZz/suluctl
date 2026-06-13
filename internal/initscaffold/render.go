package initscaffold

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type RenderOptions struct {
	Dir     string
	Package string
	Force   bool
	DryRun  bool
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
			if eerr := tpl.Execute(&buf, struct{ Package string }{opt.Package}); eerr != nil {
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
