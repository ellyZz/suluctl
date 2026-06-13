package initscaffold

import (
	"os"
	"path/filepath"
	"strings"
)

type PatchResult struct {
	Path    string
	Changed bool
	Printed string // non-empty when the user must paste this manually
}

var pkgJSONDeps = []struct{ key, line string }{
	{`"@playwright/test"`, `    "@playwright/test": "^1.53.0",`},
	{`"allure-playwright"`, `    "allure-playwright": "3.10.0",`},
	{`"allure-js-commons"`, `    "allure-js-commons": "3.10.0",`},
}

const csprojItemGroup = `  <ItemGroup>
    <PackageReference Include="Allure.Net.Commons" Version="2.15.0" />
    <None Include="allureConfig.json" CopyToOutputDirectory="PreserveNewest" />
  </ItemGroup>
`

func PatchManifest(fw Framework, dir string, dryRun bool) (PatchResult, error) {
	switch fw.Manifest {
	case patchPackageJSON:
		return patchPackageJSONFile(filepath.Join(dir, "package.json"), fw.ManifestSnippet, dryRun)
	case patchCsproj:
		return patchCsprojFile(dir, fw.ManifestSnippet, dryRun)
	default: // printOnly
		return PatchResult{Printed: fw.ManifestSnippet}, nil
	}
}

func patchPackageJSONFile(path, snippet string, dryRun bool) (PatchResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PatchResult{Printed: snippet}, nil // no package.json -> print
	}
	s := string(data)
	idx := strings.Index(s, `"devDependencies"`)
	if idx < 0 {
		return PatchResult{Printed: snippet}, nil
	}
	brace := strings.Index(s[idx:], "{")
	if brace < 0 {
		return PatchResult{Printed: snippet}, nil
	}
	at := idx + brace + 1 // just after the '{'

	var toAdd []string
	for _, dep := range pkgJSONDeps {
		if !strings.Contains(s, dep.key) {
			toAdd = append(toAdd, dep.line)
		}
	}
	if len(toAdd) == 0 {
		return PatchResult{Path: path, Changed: false}, nil // all deps already present
	}
	body := strings.Join(toAdd, "\n")
	if firstNonSpaceIsBrace(s[at:]) { // empty "devDependencies": {} — drop the trailing comma so the JSON stays valid
		body = strings.TrimRight(body, ",")
	}
	patched := s[:at] + "\n" + body + s[at:]
	if !dryRun {
		if werr := os.WriteFile(path, []byte(patched), 0o644); werr != nil {
			return PatchResult{}, werr
		}
	}
	return PatchResult{Path: path, Changed: true}, nil
}

func firstNonSpaceIsBrace(s string) bool {
	return strings.HasPrefix(strings.TrimLeft(s, " \t\r\n"), "}")
}

func patchCsprojFile(dir, snippet string, dryRun bool) (PatchResult, error) {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.csproj"))
	if len(matches) != 1 {
		return PatchResult{Printed: snippet}, nil // none or ambiguous -> print
	}
	path := matches[0]
	data, err := os.ReadFile(path)
	if err != nil {
		return PatchResult{Printed: snippet}, nil
	}
	s := string(data)
	if strings.Contains(s, "Allure.Net.Commons") {
		return PatchResult{Path: path, Changed: false}, nil
	}
	end := strings.LastIndex(s, "</Project>")
	if end < 0 {
		return PatchResult{Printed: snippet}, nil
	}
	patched := s[:end] + csprojItemGroup + s[end:]
	if !dryRun {
		if werr := os.WriteFile(path, []byte(patched), 0o644); werr != nil {
			return PatchResult{}, werr
		}
	}
	return PatchResult{Path: path, Changed: true}, nil
}
