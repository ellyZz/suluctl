package initscaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTestNGSubstitutesPackageAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	fw := Registry(TestNG)
	opt := RenderOptions{Dir: dir, Package: "com.acme.qa"}

	actions, err := Render(fw, opt)
	if err != nil {
		t.Fatal(err)
	}
	listener := filepath.Join(dir, "src/test/java/com/acme/qa/SuluLabelListener.java")
	data, err := os.ReadFile(listener)
	if err != nil {
		t.Fatalf("listener not written: %v", err)
	}
	if !strings.Contains(string(data), "package com.acme.qa;") {
		t.Errorf("package not substituted:\n%s", data)
	}
	if !strings.Contains(string(data), "suluctl-glue: v1") {
		t.Errorf("version stamp missing")
	}
	spi, _ := os.ReadFile(filepath.Join(dir, "src/test/resources/META-INF/services/org.testng.ITestNGListener"))
	if !strings.Contains(string(spi), "com.acme.qa.SuluLabelListener") {
		t.Errorf("SPI class not substituted:\n%s", spi)
	}
	if !hasVerb(actions, listener, "create") {
		t.Errorf("expected create action for listener; got %+v", actions)
	}

	actions2, err := Render(fw, opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range actions2 {
		if a.Verb == "create" || a.Verb == "overwrite" {
			t.Errorf("re-render mutated %s (verb %q)", a.Path, a.Verb)
		}
	}
}

func TestRenderDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(Registry(Pytest), RenderOptions{Dir: dir, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sulu_pytest.py")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote a file")
	}
}

// appenderMarkers are the flavor-distinguishing lines of each SuluLogAppender template.
var appenderMarkers = map[LogFlavor]string{
	LogLog4j2:  `@Plugin(name = "SuluLog"`,
	LogLogback: "extends AppenderBase<ILoggingEvent>",
}

func TestRenderTestNGWithLogsScaffoldsAppenderAndFlush(t *testing.T) {
	for _, flavor := range []LogFlavor{LogLog4j2, LogLogback} {
		t.Run(string(flavor), func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Render(Registry(TestNG), RenderOptions{Dir: dir, Package: "com.acme.qa", Logs: flavor}); err != nil {
				t.Fatal(err)
			}
			appender := filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java")
			ab, err := os.ReadFile(appender)
			if err != nil {
				t.Fatalf("appender not written for %s: %v", flavor, err)
			}
			for _, want := range []string{"package com.acme.qa;", appenderMarkers[flavor], "drainCurrentThread"} {
				if !strings.Contains(string(ab), want) {
					t.Errorf("appender missing %q", want)
				}
			}
			lb, _ := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLabelListener.java"))
			if !strings.Contains(string(lb), `Allure.addAttachment("log", "text/plain"`) {
				t.Errorf("listener afterInvocation flush missing:\n%s", lb)
			}
		})
	}
}

func TestRenderTestNGWithoutLogsOmitsAppender(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(Registry(TestNG), RenderOptions{Dir: dir, Package: "com.acme.qa"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java")); !os.IsNotExist(err) {
		t.Error("appender must NOT be scaffolded when Logs is LogNone")
	}
	lb, _ := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLabelListener.java"))
	if strings.Contains(string(lb), "addAttachment") {
		t.Errorf("listener must stay a no-op when Logs is LogNone:\n%s", lb)
	}
}

func TestRenderJUnit5WithLogsScaffoldsAppenderAndFlush(t *testing.T) {
	for _, flavor := range []LogFlavor{LogLog4j2, LogLogback} {
		t.Run(string(flavor), func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Render(Registry(JUnit5), RenderOptions{Dir: dir, Package: "com.acme.qa", Logs: flavor}); err != nil {
				t.Fatal(err)
			}
			ab, err := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java"))
			if err != nil {
				t.Fatalf("junit5 appender not written for %s: %v", flavor, err)
			}
			if !strings.Contains(string(ab), appenderMarkers[flavor]) {
				t.Errorf("appender missing %q", appenderMarkers[flavor])
			}
			ext, _ := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluAllureExtension.java"))
			s := string(ext)
			if !strings.Contains(s, "AfterTestExecutionCallback") || !strings.Contains(s, `Allure.addAttachment("log", "text/plain"`) {
				t.Errorf("junit5 extension afterTestExecution flush missing:\n%s", s)
			}
		})
	}
}

func TestRenderJUnit5WithoutLogsOmitsAppender(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(Registry(JUnit5), RenderOptions{Dir: dir, Package: "com.acme.qa"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java")); !os.IsNotExist(err) {
		t.Error("junit5 appender must NOT be scaffolded when Logs is LogNone")
	}
	ext, _ := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluAllureExtension.java"))
	if strings.Contains(string(ext), "AfterTestExecutionCallback") {
		t.Errorf("extension must not implement AfterTestExecutionCallback when Logs is LogNone")
	}
}

// Only the requested flavor's appender is written — both render to the same path,
// so a leaked marker would silently ship an appender that cannot compile.
func TestRenderLogFlavorsAreMutuallyExclusive(t *testing.T) {
	for _, fw := range []Kind{TestNG, JUnit5} {
		for flavor, marker := range appenderMarkers {
			t.Run(string(fw)+"/"+string(flavor), func(t *testing.T) {
				dir := t.TempDir()
				if _, err := Render(Registry(fw), RenderOptions{Dir: dir, Package: "com.acme.qa", Logs: flavor}); err != nil {
					t.Fatal(err)
				}
				ab, err := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java"))
				if err != nil {
					t.Fatal(err)
				}
				for other, otherMarker := range appenderMarkers {
					if other != flavor && strings.Contains(string(ab), otherMarker) {
						t.Errorf("%s render leaked the %s appender", flavor, other)
					}
				}
				if !strings.Contains(string(ab), marker) {
					t.Errorf("missing own marker %q", marker)
				}
			})
		}
	}
}

// Cross-repo contract lock (spec §5.6 / unified-ingest-runbook §13): Sulu routes an
// Allure attachment into log_events only when the name matches ^(log|logs|stdout|stderr)…$
// AND the MIME is exactly "text/plain". Changing any of these three constants here
// silently breaks per-test logs until the backend is changed in lock step.
func TestRenderLogAttachmentContractIsLocked(t *testing.T) {
	glue := map[Kind]string{
		TestNG: "src/test/java/com/acme/qa/SuluLabelListener.java",
		JUnit5: "src/test/java/com/acme/qa/SuluAllureExtension.java",
	}
	for fw, rel := range glue {
		for _, flavor := range []LogFlavor{LogLog4j2, LogLogback} {
			t.Run(string(fw)+"/"+string(flavor), func(t *testing.T) {
				dir := t.TempDir()
				if _, err := Render(Registry(fw), RenderOptions{Dir: dir, Package: "com.acme.qa", Logs: flavor}); err != nil {
					t.Fatal(err)
				}
				g, err := os.ReadFile(filepath.Join(dir, rel))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(g), `Allure.addAttachment("log", "text/plain", log, ".txt")`) {
					t.Errorf("attachment contract drifted (want name=log, MIME text/plain, ext .txt):\n%s", g)
				}
			})
		}
	}
}

// The marker strip must not chew on a user's own package path. Two guards:
// the historical "_logs" segment (which the pre-flavor marker could collide with),
// and a segment that literally spells a current marker — only reachable because
// --package is unvalidated, and only harmless because the strip runs BEFORE
// __PKG__ substitution. Move the strip after substitution and this goes red.
func TestRenderWithLogsPreservesMarkerLikePackageSegments(t *testing.T) {
	for _, tc := range []struct{ pkg, wantDir string }{
		{"com.acme._logs.qa", "src/test/java/com/acme/_logs/qa"},
		{"com.acme._logs-logback.qa", "src/test/java/com/acme/_logs-logback/qa"},
	} {
		t.Run(tc.pkg, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Render(Registry(TestNG), RenderOptions{Dir: dir, Package: tc.pkg, Logs: LogLog4j2}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, tc.wantDir, "SuluLogAppender.java")); err != nil {
				t.Errorf("appender path corrupted by the log-marker strip: %v", err)
			}
		})
	}
}

func hasVerb(actions []Action, absPath, verb string) bool {
	for _, a := range actions {
		if strings.HasSuffix(absPath, a.Path) && a.Verb == verb {
			return true
		}
	}
	return false
}
