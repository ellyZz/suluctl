# suluctl log capture — Track O2 (`init` scaffolds per-framework log→Allure glue) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `suluctl init` scaffolds per-test log capture for Java (log4j2) test suites — a custom log4j2 appender + a per-test listener/extension flush that attaches a `log`/`text-plain` Allure attachment, which Sulu's importer (PR #152) routes into each result's Logs panel.

**Architecture:** Extend the existing `internal/initscaffold` scaffolder. A new render option `WithLogs` drives a conditional `_logs/` template path (rendered only when enabled) plus `{{if .WithLogs}}` blocks in the already-scaffolded `sulu_id` glue (TestNG `SuluLabelListener`, JUnit5 `SuluAllureExtension`). `init` computes `WithLogs` by **auto-detecting log4j2** in the project's build file (Java frameworks only) — so logback/JUL projects are never handed a non-compiling appender. pytest/Playwright logs are already captured by their Allure plugins (doc note only); xUnit is deferred to v0.4.

**Tech Stack:** Go 1.25 (stdlib `text/template`, `embed`, `strings`); scaffolded artifacts are Java (log4j2 `log4j-core` `@Plugin` appender + TestNG/JUnit5 listeners). No new Go deps. No backend change (rides the shipped PR #152 routing).

## Global Constraints

- **Module** `github.com/ellyZz/suluctl`, **Go 1.25**, stdlib only (no new deps).
- **Cross-repo contract (must not drift) — Sulu importer routing:** the scaffolded glue emits exactly one per-test Allure attachment: **name `log`, MIME exactly `text/plain` (bare — NO `; charset=…`), file ext `.txt`**, with a `PatternLayout` carrying a leading `HH:mm:ss.SSS` time + level. These three constants are locked against the backend `LogAttachmentParser.isLogAttachment` regex `^(log|logs|stdout|stderr)(\.(txt|log))?$` + exact-equals MIME.
- **log4j2-specific (documented v1 limit):** the appender extends `org.apache.logging.log4j.core.appender.AbstractAppender` — it requires `log4j-core` on the test classpath. `init` scaffolds it **only when log4j2 is detected**; logback/JUL support is a follow-up.
- **Per-thread capture limit (documented):** the appender buffers per `ThreadLocal`, so only the **test thread's** log lines are attached; non-test-thread logs (executors, async) are out of scope for v1 (MDC-`sulu_id` routing is the upgrade path).
- **Non-destructive:** when `WithLogs` is false, `init` renders exactly as today (the `sulu_id` glue is byte-identical; no `_logs/` files; listener flush blocks empty).
- **Plugin discovery:** the log4j2 `<Configuration>` must carry `packages="<glue package>"` so the custom `<SuluLog>` element resolves (annotation-processor discovery is not guaranteed). This is emitted as a printed Manual step.
- **Every task ends green:** `gofmt -l` clean, `go vet ./...` clean, `go test ./... -race` passing.

---

### Task 1: `WithLogs` render plumbing + TestNG log glue

**Files:**
- Modify: `internal/initscaffold/render.go` (add `RenderOptions.WithLogs`; conditional `_logs/` path; template data gains `WithLogs`)
- Create: `internal/initscaffold/templates/testng/src/test/java/__PKG__/_logs/SuluLogAppender.java.tmpl`
- Modify: `internal/initscaffold/templates/testng/src/test/java/__PKG__/SuluLabelListener.java.tmpl` (fill `afterInvocation` under `{{if .WithLogs}}`)
- Modify: `internal/initscaffold/embed_test.go` (assert the new appender path), `internal/initscaffold/render_test.go` (add a WithLogs render test)

**Interfaces:**
- Produces: `initscaffold.RenderOptions.WithLogs bool`; when true for TestNG, `Render` writes `src/test/java/<pkg>/SuluLogAppender.java` and the listener's `afterInvocation` flushes a `log`/`text-plain` attachment. When false, neither appears.

- [ ] **Step 1: Write the failing render test**

Add to `internal/initscaffold/render_test.go`:

```go
func TestRenderTestNGWithLogsScaffoldsAppenderAndFlush(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(Registry(TestNG), RenderOptions{Dir: dir, Package: "com.acme.qa", WithLogs: true}); err != nil {
		t.Fatal(err)
	}
	appender := filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java")
	ab, err := os.ReadFile(appender)
	if err != nil {
		t.Fatalf("appender not written with WithLogs: %v", err)
	}
	for _, want := range []string{"package com.acme.qa;", "@Plugin(name = \"SuluLog\"", "drainCurrentThread"} {
		if !strings.Contains(string(ab), want) {
			t.Errorf("appender missing %q", want)
		}
	}
	lb, _ := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLabelListener.java"))
	if !strings.Contains(string(lb), `Allure.addAttachment("log", "text/plain"`) {
		t.Errorf("listener afterInvocation flush missing:\n%s", lb)
	}
}

func TestRenderTestNGWithoutLogsOmitsAppender(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(Registry(TestNG), RenderOptions{Dir: dir, Package: "com.acme.qa"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java")); !os.IsNotExist(err) {
		t.Error("appender must NOT be scaffolded when WithLogs is false")
	}
	lb, _ := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLabelListener.java"))
	if strings.Contains(string(lb), "addAttachment") {
		t.Errorf("listener must stay a no-op when WithLogs is false:\n%s", lb)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /Users/ruslan/projects/aisulu_dev/suluctl && go test ./internal/initscaffold/ -run WithLogs -v`
Expected: FAIL — `RenderOptions.WithLogs` is an unknown field (compile error).

- [ ] **Step 3: Add `WithLogs` to render.go**

In `internal/initscaffold/render.go`, add the field to `RenderOptions`:

```go
type RenderOptions struct {
	Dir      string
	Package  string
	Force    bool
	DryRun   bool
	WithLogs bool
}
```

In `Render`, inside the `fs.WalkDir` callback, **after** `rel := strings.TrimPrefix(p, root+"/")` and the `__PKG__` substitution, add the `_logs/` handling (skip when disabled, strip the marker segment when enabled):

```go
		if strings.Contains(rel, "_logs/") {
			if !opt.WithLogs {
				return nil // log-only glue, not requested
			}
			rel = strings.Replace(rel, "_logs/", "", 1)
		}
```

Change the template data struct to carry `WithLogs`:

```go
			if eerr := tpl.Execute(&buf, struct {
				Package  string
				WithLogs bool
			}{opt.Package, opt.WithLogs}); eerr != nil {
				return eerr
			}
```

- [ ] **Step 4: Create the appender template**

Create `internal/initscaffold/templates/testng/src/test/java/__PKG__/_logs/SuluLogAppender.java.tmpl`:

```java
package {{.Package}};

import io.qameta.allure.Allure;
import org.apache.logging.log4j.core.Appender;
import org.apache.logging.log4j.core.Core;
import org.apache.logging.log4j.core.Layout;
import org.apache.logging.log4j.core.LogEvent;
import org.apache.logging.log4j.core.appender.AbstractAppender;
import org.apache.logging.log4j.core.config.Property;
import org.apache.logging.log4j.core.config.plugins.Plugin;
import org.apache.logging.log4j.core.config.plugins.PluginAttribute;
import org.apache.logging.log4j.core.config.plugins.PluginElement;
import org.apache.logging.log4j.core.config.plugins.PluginFactory;
import org.apache.logging.log4j.core.layout.PatternLayout;

import java.io.Serializable;

/**
 * log4j2 appender that buffers each test thread's rendered log output so the Sulu
 * glue can attach it per test. Drained once per test by the listener/extension via
 * {@link #drainCurrentThread()}. Registered in log4j2.xml as &lt;SuluLog/&gt; under a
 * &lt;Configuration packages="{{.Package}}"&gt; so the plugin is discoverable.
 */
@Plugin(name = "SuluLog", category = Core.CATEGORY_NAME, elementType = Appender.ELEMENT_TYPE)
public final class SuluLogAppender extends AbstractAppender {

    // Per-thread buffer: parallel @Test methods each accumulate their own log.
    private static final ThreadLocal<StringBuilder> BUF = ThreadLocal.withInitial(StringBuilder::new);

    private SuluLogAppender(final String name, final Layout<? extends Serializable> layout) {
        super(name, null, layout, true, Property.EMPTY_ARRAY);
    }

    @PluginFactory
    public static SuluLogAppender create(
            @PluginAttribute("name") final String name,
            @PluginElement("Layout") Layout<? extends Serializable> layout) {
        if (layout == null) {
            layout = PatternLayout.newBuilder()
                    .withPattern("%d{HH:mm:ss.SSS} %-5level %logger{1} - %msg%n").build();
        }
        return new SuluLogAppender(name == null ? "SuluLog" : name, layout);
    }

    @Override
    public void append(final LogEvent event) {
        BUF.get().append(new String(getLayout().toByteArray(event)));
    }

    /** Returns and clears this thread's buffered log. */
    public static String drainCurrentThread() {
        final StringBuilder sb = BUF.get();
        final String out = sb.toString();
        sb.setLength(0);
        return out;
    }
}
```

- [ ] **Step 5: Fill the listener `afterInvocation` under `{{if .WithLogs}}`**

In `internal/initscaffold/templates/testng/src/test/java/__PKG__/SuluLabelListener.java.tmpl`, replace the `afterInvocation` body:

```java
    @Override
    public void afterInvocation(final IInvokedMethod method, final ITestResult testResult) {
{{- if .WithLogs}}
        if (!method.isTestMethod()) {
            return;
        }
        final String log = SuluLogAppender.drainCurrentThread();
        if (!log.isBlank()) {
            // Sulu importer contract (PR #152): name=log, MIME exactly text/plain, ext .txt
            Allure.addAttachment("log", "text/plain", log, ".txt");
        }
{{- else}}
        // No-op: allure-testng owns status/stop. We only label on the way in.
{{- end}}
    }
```

(`Allure` is already imported in this template; `SuluLogAppender` is same-package.)

- [ ] **Step 6: Add the appender to the embed assertions**

In `internal/initscaffold/embed_test.go`, add the appender path to the `TestNG` entry of `wantPaths`:

```go
		TestNG:     {"templates/testng/src/test/java/__PKG__/SuluLabelListener.java.tmpl", "templates/testng/src/test/resources/META-INF/services/org.testng.ITestNGListener.tmpl", "templates/testng/src/test/java/__PKG__/_logs/SuluLogAppender.java.tmpl"},
```

- [ ] **Step 7: Run tests + vet + gofmt**

Run: `go test ./internal/initscaffold/ -race -v && go vet ./internal/initscaffold/ && gofmt -l internal/initscaffold/`
Expected: PASS (new WithLogs tests + existing `TestRenderTestNGSubstitutesPackageAndIsIdempotent` etc.), no vet/gofmt output.

- [ ] **Step 8: Commit**

```bash
git add internal/initscaffold/render.go internal/initscaffold/templates/testng/ internal/initscaffold/embed_test.go internal/initscaffold/render_test.go
git commit -m "feat(init): WithLogs render plumbing + TestNG log4j2 log glue"
```

---

### Task 2: JUnit5 log glue (appender + `afterTestExecution` flush)

**Files:**
- Create: `internal/initscaffold/templates/junit5/src/test/java/__PKG__/_logs/SuluLogAppender.java.tmpl` (identical content to the TestNG appender — same log4j2 plugin; the two live in separate template trees because `Render` walks per-framework dirs)
- Modify: `internal/initscaffold/templates/junit5/src/test/java/__PKG__/SuluAllureExtension.java.tmpl` (add `AfterTestExecutionCallback` + flush under `{{if .WithLogs}}`)
- Modify: `internal/initscaffold/embed_test.go` (assert the JUnit5 appender path), `internal/initscaffold/render_test.go` (add a JUnit5 WithLogs test)

**Interfaces:**
- Consumes: `RenderOptions.WithLogs` + the `_logs/` mechanism from Task 1.
- Produces: JUnit5 projects with `WithLogs` get the same per-test `log`/`text-plain` attachment via the extension's `afterTestExecution`.

- [ ] **Step 1: Write the failing render test**

Add to `internal/initscaffold/render_test.go`:

```go
func TestRenderJUnit5WithLogsScaffoldsAppenderAndFlush(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(Registry(JUnit5), RenderOptions{Dir: dir, Package: "com.acme.qa", WithLogs: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java")); err != nil {
		t.Fatalf("junit5 appender not written: %v", err)
	}
	ext, _ := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluAllureExtension.java"))
	s := string(ext)
	if !strings.Contains(s, "AfterTestExecutionCallback") || !strings.Contains(s, `Allure.addAttachment("log", "text/plain"`) {
		t.Errorf("junit5 extension afterTestExecution flush missing:\n%s", s)
	}
}

func TestRenderJUnit5WithoutLogsOmitsAppender(t *testing.T) {
	dir := t.TempDir()
	if _, err := Render(Registry(JUnit5), RenderOptions{Dir: dir, Package: "com.acme.qa"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java")); !os.IsNotExist(err) {
		t.Error("junit5 appender must NOT be scaffolded when WithLogs is false")
	}
	ext, _ := os.ReadFile(filepath.Join(dir, "src/test/java/com/acme/qa/SuluAllureExtension.java"))
	if strings.Contains(string(ext), "AfterTestExecutionCallback") {
		t.Errorf("extension must not implement AfterTestExecutionCallback when WithLogs is false")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/initscaffold/ -run JUnit5WithLogs -v` (and `-run JUnit5Without`)
Expected: FAIL — appender file absent / `AfterTestExecutionCallback` not present.

- [ ] **Step 3: Create the JUnit5 appender template**

Create `internal/initscaffold/templates/junit5/src/test/java/__PKG__/_logs/SuluLogAppender.java.tmpl` with **the exact same content** as the TestNG appender from Task 1 Step 4 (same `package {{.Package}};`, same class). It is duplicated because `Render` walks each framework's own template tree; document this in the JUnit5 appender's class comment with one line: `// (identical to the TestNG glue's appender — separate file because init renders per framework)`.

- [ ] **Step 4: Add the flush to the extension under `{{if .WithLogs}}`**

In `internal/initscaffold/templates/junit5/src/test/java/__PKG__/SuluAllureExtension.java.tmpl`:

Change the class declaration to conditionally implement the after-callback (FQN avoids a conditional import):

```java
public final class SuluAllureExtension implements BeforeTestExecutionCallback{{if .WithLogs}}, org.junit.jupiter.api.extension.AfterTestExecutionCallback{{end}} {
```

Add the flush method just before the closing brace of the class:

```java
{{- if .WithLogs}}

    @Override
    public void afterTestExecution(final ExtensionContext context) {
        final String log = SuluLogAppender.drainCurrentThread();
        if (!log.isBlank()) {
            // Sulu importer contract (PR #152): name=log, MIME exactly text/plain, ext .txt
            Allure.addAttachment("log", "text/plain", log, ".txt");
        }
    }
{{- end}}
```

(`Allure` and `ExtensionContext` are already imported in this template; `SuluLogAppender` is same-package.)

- [ ] **Step 5: Add the JUnit5 appender to embed assertions**

In `internal/initscaffold/embed_test.go`, add to the `JUnit5` entry of `wantPaths`:

```go
		JUnit5:     {"templates/junit5/src/test/java/__PKG__/SuluAllureExtension.java.tmpl", "templates/junit5/src/test/resources/allure.properties", "templates/junit5/src/test/java/__PKG__/_logs/SuluLogAppender.java.tmpl"},
```

- [ ] **Step 6: Run tests + vet + gofmt**

Run: `go test ./internal/initscaffold/ -race -v && go vet ./internal/initscaffold/ && gofmt -l internal/initscaffold/`
Expected: PASS (the 2 new JUnit5 tests + Task 1's + existing), no vet/gofmt output.

- [ ] **Step 7: Commit**

```bash
git add internal/initscaffold/templates/junit5/ internal/initscaffold/embed_test.go internal/initscaffold/render_test.go
git commit -m "feat(init): JUnit5 log4j2 log glue (afterTestExecution flush)"
```

---

### Task 3: log4j2 auto-detection + `init` wiring + Manual steps

**Files:**
- Modify: `internal/initscaffold/detect.go` (add `DetectLog4j2`)
- Create: `internal/initscaffold/log4j2.go` (the `Log4j2SetupSteps` + `Log4j2HintSteps` helpers)
- Modify: `internal/cmd/init.go` (compute `withLogs`, pass to `Render`, append the conditional Manual steps)
- Test: `internal/initscaffold/detect_test.go` (detection), `internal/cmd/init_test.go` (TestNG-with-log4j2 integration)

**Interfaces:**
- Consumes: `RenderOptions.WithLogs` (Task 1).
- Produces: `initscaffold.DetectLog4j2(dir string) bool`; `initscaffold.Log4j2SetupSteps(pkg string) []string`; `initscaffold.Log4j2HintSteps() []string`. `init` scaffolds the log glue when a Java framework's build file references log4j2, and prints the registration steps; when a Java framework lacks log4j2 it prints a one-line "add log4j2 + re-run" hint.

- [ ] **Step 1: Write the failing detection test**

Add to `internal/initscaffold/detect_test.go`:

```go
func TestDetectLog4j2(t *testing.T) {
	dir := t.TempDir()
	if DetectLog4j2(dir) {
		t.Error("empty dir must not detect log4j2")
	}
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"),
		[]byte("dependencies { testImplementation 'org.apache.logging.log4j:log4j-core:2.23.1' }"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !DetectLog4j2(dir) {
		t.Error("build.gradle with log4j-core must detect log4j2")
	}
}
```

(Ensure `os` and `path/filepath` are imported in `detect_test.go`.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/initscaffold/ -run DetectLog4j2 -v`
Expected: FAIL — `DetectLog4j2` undefined.

- [ ] **Step 3: Implement detection**

Add to `internal/initscaffold/detect.go` (uses the existing `readIfAny` + `strings`):

```go
// DetectLog4j2 reports whether the project's build file references log4j2-core,
// which the scaffolded SuluLogAppender requires to compile. Best-effort grep.
func DetectLog4j2(dir string) bool {
	build := readIfAny(dir, "build.gradle", "build.gradle.kts", "pom.xml")
	return strings.Contains(build, "log4j-core") || strings.Contains(build, "org.apache.logging.log4j")
}
```

- [ ] **Step 4: Add the Manual-step helpers**

Create `internal/initscaffold/log4j2.go`:

```go
package initscaffold

// Log4j2SetupSteps are the printed manual steps for registering the scaffolded
// SuluLog appender — emitted only when log4j2 is detected and the glue was written.
func Log4j2SetupSteps(pkg string) []string {
	if pkg == "" {
		pkg = "<your glue package>"
	}
	return []string{
		"Log capture: ensure log4j-core is a test dependency (e.g. testImplementation 'org.apache.logging.log4j:log4j-core:2.23.1').",
		"Register the SuluLog appender in src/test/resources/log4j2.xml:\n" +
			"    set <Configuration packages=\"" + pkg + "\"> so log4j2 resolves the custom element,\n" +
			"    add <SuluLog name=\"SuluLog\"><PatternLayout pattern=\"%d{HH:mm:ss.SSS} %-5level %logger{1} - %msg%n\"/></SuluLog>,\n" +
			"    then add <AppenderRef ref=\"SuluLog\"/> inside <Root>.",
	}
}

// Log4j2HintSteps tell a Java project WITHOUT log4j2 how to opt into per-test logs.
func Log4j2HintSteps() []string {
	return []string{
		"Per-test logs: add log4j-core (log4j2) as a test dependency and re-run `suluctl init --force` to scaffold the SuluLog appender.",
	}
}
```

- [ ] **Step 5: Write the failing `init` integration test**

Add to `internal/cmd/init_test.go`:

```go
func TestInitTestNGWithLog4j2ScaffoldsLogGlue(t *testing.T) {
	neutralizeEnv(t)
	dir := t.TempDir()
	// a TestNG project that uses log4j2
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"),
		[]byte("dependencies {\n  testImplementation 'org.testng:testng:7.10.2'\n  testImplementation 'org.apache.logging.log4j:log4j-core:2.23.1'\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out, errB bytes.Buffer
	code := Init([]string{"--package", "com.acme.qa"}, &out, &errB, "test")
	if code != 0 {
		t.Fatalf("exit %d; stderr=%s", code, errB.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "src/test/java/com/acme/qa/SuluLogAppender.java")); err != nil {
		t.Errorf("SuluLogAppender not scaffolded for a log4j2 TestNG project: %v", err)
	}
	if !strings.Contains(out.String(), "<SuluLog") {
		t.Errorf("report must print the log4j2.xml registration step:\n%s", out.String())
	}
}
```

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/cmd/ -run InitTestNGWithLog4j2 -v`
Expected: FAIL — appender not scaffolded / registration step not printed (init doesn't compute `withLogs` yet).

- [ ] **Step 7: Wire `init.go`**

In `internal/cmd/init.go`, after the `pkg` is resolved (`if fw.JavaPackage && pkg == "" { pkg = initscaffold.DetectJavaBasePackage(dir) }`), compute `withLogs` and pass it to `Render`:

```go
	withLogs := fw.JavaPackage && initscaffold.DetectLog4j2(dir)

	actions, err := initscaffold.Render(fw, initscaffold.RenderOptions{
		Dir: dir, Package: pkg, Force: force, DryRun: dryRun, WithLogs: withLogs,
	})
```

Then, after `PatchManifest` and before `PrintReport`, append the conditional Manual steps:

```go
	if withLogs {
		fw.ManualSteps = append(fw.ManualSteps, initscaffold.Log4j2SetupSteps(pkg)...)
	} else if fw.JavaPackage {
		fw.ManualSteps = append(fw.ManualSteps, initscaffold.Log4j2HintSteps()...)
	}
```

(`fw` is a local value from `Registry(kind)`, so appending does not mutate the registry. `PrintReport` already renders `fw.ManualSteps`.)

- [ ] **Step 8: Run the focused tests + the package suites**

Run: `go test ./internal/initscaffold/ ./internal/cmd/ -race -v && go vet ./internal/initscaffold/ ./internal/cmd/ && gofmt -l internal/`
Expected: PASS (detection test, init-with-log4j2 test, all existing init/initscaffold tests — the Playwright/pytest inits are unaffected because `fw.JavaPackage` is false for them), no vet/gofmt output.

- [ ] **Step 9: Commit**

```bash
git add internal/initscaffold/detect.go internal/initscaffold/log4j2.go internal/initscaffold/detect_test.go internal/cmd/init.go internal/cmd/init_test.go
git commit -m "feat(init): auto-detect log4j2 -> scaffold log glue + print registration steps"
```

---

### Task 4: pytest/Playwright note + README/docs

**Files:**
- Modify: `internal/initscaffold/scaffold.go` (one `ManualSteps` line for pytest + Playwright noting logs are auto-captured)
- Modify: `README.md` (document `init` log capture + the contract + limits)
- Test: rely on the suite staying green (the new ManualSteps lines are additive prose; `init_test.go`'s Playwright E2E asserts a `Contains`, not an exact report, so it stays green)

**Interfaces:** none (docs + report prose).

- [ ] **Step 1: Add the pytest/Playwright auto-capture note**

In `internal/initscaffold/scaffold.go` `Registry`, append one `ManualSteps` entry to the **Pytest** and **Playwright** cases (these frameworks' Allure plugins already attach captured output):

- Pytest case — append to its `ManualSteps`:
  ```go
  "Per-test logs: allure-pytest already attaches captured stdout/stderr/log as text/plain — enable pytest log capture (e.g. log_cli or default capture) and they appear in Sulu's Logs panel.",
  ```
- Playwright case — append to its `ManualSteps`:
  ```go
  "Per-test logs: allure-playwright already attaches captured stdout/stderr — no extra setup; they appear in Sulu's Logs panel.",
  ```

- [ ] **Step 2: Document in README**

In `README.md`, under the `suluctl init` documentation, add a subsection:

```markdown
### Per-test logs (init)

`suluctl init` can also wire **per-test** log capture so each test's log output
shows in that result's Logs panel in Sulu.

- **Java (log4j2):** when your build uses log4j2, `init` scaffolds a `SuluLogAppender`
  and a per-test flush, and prints the `log4j2.xml` registration to add (a
  `<SuluLog>` appender + `<Configuration packages="…">`). Requires `log4j-core`.
  (logback/JUL are not auto-wired yet — add log4j2 or capture manually.)
- **pytest / Playwright:** logs are already captured by `allure-pytest` /
  `allure-playwright` — `init` just reminds you to enable capture.
- **xUnit:** per-test log capture is planned for a later release.

> ℹ️ This is **per-test** (each result's Logs panel). For the **whole-run console**
> regardless of framework, use `suluctl watch` (see *Console logs* above).

> Capture is per **test thread** — logs emitted on other threads (executors, async
> callbacks) are not attached in this version.
```

- [ ] **Step 3: Whole-suite verification**

Run: `gofmt -l . && go vet ./... && go test ./... -race`
Expected: no `gofmt`/`vet` output; all packages PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/initscaffold/scaffold.go README.md
git commit -m "docs(init): per-test log capture — pytest/playwright note + README"
```

---

## Self-Review

**Spec coverage (§5 → task):**
- §5.2(a) log4j2 `SuluLogAppender` template → Task 1 (TestNG) + Task 2 (JUnit5).
- §5.2(b) per-test listener/extension flush (`afterInvocation` / `afterTestExecution`) emitting `log`/`text-plain`/`.txt` → Task 1 + Task 2.
- §5.2(c) Registry/Manual steps (log4j-core dep + log4j2.xml registration) → Task 3 (`Log4j2SetupSteps`).
- §5.3 plugin discovery (`packages="<pkg>"` on `<Configuration>`; scaffold-if-absent / print-if-present) → Task 3 (printed registration step; the appender Java is scaffolded, the XML registration is printed because init never merges arbitrary XML — consistent with the existing `printOnly`/`ManualSteps` pattern).
- §5.4 pytest/Playwright near-free (doc note) → Task 4.
- §5.5 xUnit deferred → Task 4 README note (no code).
- §5.6 cross-repo contract constants (name `log`, bare `text/plain`, `.txt`, CONSOLE-identical layout) → Global Constraints + asserted in Task 1/Task 2 render tests (`Allure.addAttachment("log", "text/plain"`) + the appender's `PatternLayout`.
- **Design decision (resolved beyond the spec):** auto-detect log4j2 (Task 3) rather than always-scaffold — avoids handing a non-compiling appender to logback/JUL Java projects; non-detected Java projects get `Log4j2HintSteps`. Documented in the plan's Architecture + Global Constraints.

**Placeholder scan:** none — every code/template/test step carries full content and an exact run command with expected output.

**Type consistency:** `RenderOptions.WithLogs` (Task 1) is read by `Render`'s `_logs/` skip + template data (Task 1), consumed for JUnit5 (Task 2), and set by `init` from `DetectLog4j2` (Task 3). `Log4j2SetupSteps(pkg)`/`Log4j2HintSteps()` (Task 3) are appended to `fw.ManualSteps`, which `PrintReport` already renders (`output.go:66`). The appender class name `SuluLogAppender` + method `drainCurrentThread()` match between the appender template and both listener/extension flush calls.
