package initscaffold

type Kind string

const (
	TestNG     Kind = "testng"
	JUnit5     Kind = "junit5"
	Playwright Kind = "playwright"
	Pytest     Kind = "pytest"
	XUnit      Kind = "xunit"
)

var AllKinds = []Kind{TestNG, JUnit5, Playwright, Pytest, XUnit}

type manifestMode int

const (
	patchPackageJSON manifestMode = iota
	patchCsproj
	printOnly
)

type Framework struct {
	Kind            Kind
	Display         string
	JavaPackage     bool
	ResultsDir      string
	TestCmd         string
	Manifest        manifestMode
	ManifestSnippet string
	ManualSteps     []string
}

func Registry(k Kind) Framework {
	switch k {
	case TestNG:
		return Framework{Kind: TestNG, Display: "TestNG", JavaPackage: true,
			ResultsDir: "build/allure-results", TestCmd: "./gradlew test", Manifest: printOnly,
			ManifestSnippet: "Add to build.gradle:\n" +
				"  testImplementation 'io.qameta.allure:allure-testng:2.34.0'\n" +
				"  test { systemProperty 'allure.results.directory', \"$buildDir/allure-results\" }"}
	case JUnit5:
		return Framework{Kind: JUnit5, Display: "JUnit 5", JavaPackage: true,
			ResultsDir: "build/allure-results", TestCmd: "./gradlew test", Manifest: printOnly,
			ManifestSnippet: "Add to build.gradle:\n" +
				"  testImplementation platform('io.qameta.allure:allure-bom:2.35.2')\n" +
				"  testImplementation 'io.qameta.allure:allure-jupiter'"}
	case Playwright:
		return Framework{Kind: Playwright, Display: "Playwright", JavaPackage: false,
			ResultsDir: "allure-results", TestCmd: "npx playwright test", Manifest: patchPackageJSON,
			ManifestSnippet: "Add to package.json devDependencies:\n" +
				"  \"@playwright/test\": \"^1.53.0\", \"allure-playwright\": \"3.10.0\", \"allure-js-commons\": \"3.10.0\"",
			ManualSteps: []string{
				"Add to playwright.config.ts: reporter: [['list'], ['allure-playwright', { resultsDir: 'allure-results' }]]",
				"In every spec, import test from './support/sulu' (not '@playwright/test') — the auto fixture fires only then.",
				"Run: npm install",
			}}
	// Pytest: Manifest is printOnly (NOT auto-patch) — Go stdlib has no TOML parser;
	// refines spec Decision #3 (see the spec's planning-amendment footnote).
	case Pytest:
		return Framework{Kind: Pytest, Display: "pytest", JavaPackage: false,
			ResultsDir: "allure-results", TestCmd: "python -m pytest", Manifest: printOnly,
			ManifestSnippet: "Add to pyproject.toml:\n" +
				"  [project.optional-dependencies] test += \"allure-pytest==2.16.0\"\n" +
				"  [tool.pytest.ini_options] addopts = \"... --alluredir allure-results\"",
			ManualSteps: []string{"Install: python -m pip install allure-pytest==2.16.0"}}
	case XUnit:
		return Framework{Kind: XUnit, Display: "xUnit", JavaPackage: false,
			ResultsDir: "allure-results", TestCmd: "dotnet test", Manifest: patchCsproj,
			ManifestSnippet: "Add to your .csproj:\n" +
				"  <PackageReference Include=\"Allure.Net.Commons\" Version=\"2.15.0\" />\n" +
				"  <None Include=\"allureConfig.json\" CopyToOutputDirectory=\"PreserveNewest\" />",
			ManualSteps: []string{
				"xUnit has NO auto allure binding: apply [SuluTest(\"<id>\")] (with `using Sulu.XUnit;`) to a test, or NO results are produced.",
				"If you already have an AssemblyInfo.cs, add: [assembly: CollectionBehavior(MaxParallelThreads = 1)]",
			}}
	}
	panic("initscaffold: unknown Kind " + string(k))
}
