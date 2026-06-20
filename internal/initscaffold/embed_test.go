package initscaffold

import (
	"io/fs"
	"testing"
)

func TestTemplatesEmbedded(t *testing.T) {
	wantPaths := map[Kind][]string{
		TestNG:     {"templates/testng/src/test/java/__PKG__/SuluLabelListener.java.tmpl", "templates/testng/src/test/resources/META-INF/services/org.testng.ITestNGListener.tmpl", "templates/testng/src/test/java/__PKG__/_logs/SuluLogAppender.java.tmpl"},
		JUnit5:     {"templates/junit5/src/test/java/__PKG__/SuluAllureExtension.java.tmpl", "templates/junit5/src/test/resources/allure.properties"},
		Playwright: {"templates/playwright/tests/support/sulu.ts"},
		Pytest:     {"templates/pytest/sulu_pytest.py"},
		XUnit:      {"templates/xunit/Support/SuluTestAttribute.cs", "templates/xunit/allureConfig.json", "templates/xunit/AssemblyInfo.cs"},
	}
	for k, paths := range wantPaths {
		for _, p := range paths {
			data, err := fs.ReadFile(templatesFS, p)
			if err != nil {
				t.Errorf("%s: missing embed %s: %v", k, p, err)
				continue
			}
			if len(data) == 0 {
				t.Errorf("%s: empty embed %s", k, p)
			}
		}
	}
}
