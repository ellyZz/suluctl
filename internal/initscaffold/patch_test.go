package initscaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchPackageJSONInsertsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", "{\n  \"name\": \"demo\",\n  \"devDependencies\": {\n    \"typescript\": \"^5.4.0\"\n  }\n}\n")

	res, err := PatchManifest(Registry(Playwright), dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatalf("expected a change; printed=%q", res.Printed)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	for _, want := range []string{`"allure-playwright": "3.10.0"`, `"allure-js-commons": "3.10.0"`, `"@playwright/test": "^1.53.0"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("package.json missing %s:\n%s", want, data)
		}
	}
	res2, _ := PatchManifest(Registry(Playwright), dir, false)
	if res2.Changed {
		t.Errorf("second patch changed the file again")
	}
}

func TestPatchPackageJSONEmptyDevDepsStaysValidJSON(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", "{\n  \"devDependencies\": {}\n}\n")
	if _, err := PatchManifest(Registry(Playwright), dir, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	if !json.Valid(data) {
		t.Fatalf("patched package.json is not valid JSON:\n%s", data)
	}
}

func TestPatchCsprojAppendsItemGroup(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Api.csproj", "<Project Sdk=\"Microsoft.NET.Sdk\">\n  <ItemGroup>\n    <PackageReference Include=\"xunit.v3\" Version=\"3.0.0\" />\n  </ItemGroup>\n</Project>\n")

	res, err := PatchManifest(Registry(XUnit), dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatalf("expected a change; printed=%q", res.Printed)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "Api.csproj"))
	for _, want := range []string{`Allure.Net.Commons`, `Version="2.15.0"`, `<None Include="allureConfig.json"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("csproj missing %s:\n%s", want, data)
		}
	}
}

func TestPatchPrintOnlyAndMissingAnchorFallBackToPrint(t *testing.T) {
	res, _ := PatchManifest(Registry(TestNG), t.TempDir(), false)
	if res.Changed || res.Printed == "" {
		t.Errorf("testng should print, not patch: %+v", res)
	}
	res2, _ := PatchManifest(Registry(Playwright), t.TempDir(), false)
	if res2.Changed || res2.Printed == "" {
		t.Errorf("missing package.json should fall back to print: %+v", res2)
	}
}
