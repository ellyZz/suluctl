# suluctl log capture — design

> Status: **approved (design)** · 2026-06-20 · target repo: `ellyZz/suluctl`
> Companion backend: `ellyZz/sulu` (no backend change required — both tracks ride already-shipped endpoints)

## 1. Problem

Test logs that are visible in the console / CI are **not** appearing in Sulu launches when results are reported via suluctl.

Root cause (proven this session against prod launch `74` / result `10691` and the reference suite `aisulu-prod-tests`):

- The reference suite logs only to a log4j2 `CONSOLE` + `FILE` appender; it **never attaches** log output to the Allure report. Every `*-result.json` carries `"attachments":[]`, so suluctl uploads no log payload, and the launch's Logs panel is empty (`attachments:[]`, `0` log rows on prod).
- The console/CI stdout the user sees is **not part of the uploaded report** — it only exists in the terminal.

Sulu's backend already routes log content into `log_events` when the report contains it (PR #152): an Allure attachment named `^(log|logs|stdout|stderr)(\.(txt|log))?$` with MIME **exactly** `text/plain` is routed per-test into the Logs panel (`source=allure-log-attachment`); JUnit `<system-out>`/`<system-err>` become per-test (`junit-import`) or suite-level (`junit-import-suite`) logs. The producing side simply never emits any of these.

## 2. Goal & the fundamental tradeoff

Make logs flow into Sulu launches with minimal, ideally one-time, setup — as language/framework-agnostic as physically possible.

The investigation (4-agent workflow, adversarially verified) established a hard constraint:

- **Language-agnostic + "set up once" is achievable only at *launch* granularity.** An external process (suluctl) can tee the wrapped command's console stream, but it cannot attribute lines to individual test results: there are no test-boundary events in a byte stream, parallel tests interleave on one pipe, and many frameworks capture stdout *in-process* (pytest default, Surefire `redirectTestOutputToFile`, Playwright) so the bytes never reach the OS pipe suluctl inherits.
- **Precise *per-test* logs require in-process, per-framework code** — only the framework knows test boundaries. This is not avoidable by any external-process cleverness (a per-line correlation token re-introduces per-framework instrumentation, i.e. *more* setup, and still breaks under parallelism).

Therefore the design ships **two complementary tracks**, not one:

| Track | What | Where | Agnostic | Granularity |
|---|---|---|---|---|
| **O1** | tee wrapped console → launch-scoped log | suluctl `watch` runtime | ✅ yes | launch-scoped |
| **O2** | scaffold per-framework log→Allure glue | suluctl `init` (setup-time) | ❌ per-framework | per-test |

They are framed to users as: **O1** = "every run's console in Sulu, any stack"; **O2** = "per-test logs in the Logs panel, via the framework's Allure integration."

## 3. Non-goals (YAGNI)

- **Per-test attribution from the external tee** (rejected option O3 token-routing / O4 result-JSON patching) — both re-introduce per-framework setup *and* fail under parallelism: all of O2's cost, none of its reliability.
- **Live per-tick log streaming for O1** — O1 flushes once, at child exit. Imported logs are a finished-run snapshot. (A closed/finished launch persists appended logs but does not WS-broadcast them — acceptable for a post-run flush.)
- **xUnit/.NET log capture** — deferred to the already-planned suluctl `v0.4` (`ITestOutputHelper`).
- **`upload` (one-shot) console capture** — O1 lives in `watch` only; `upload` of a pre-existing results dir has no live stream to tee.
- **No backend change, no Flyway migration** in either track.

---

## 4. Track O1 — `watch` tees console → launch-scoped logs

### 4.1 Capture

`internal/runner/runner.go` currently passes stdio straight through (`runner.go:27`: `cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr`).

Change `runner.Run` to accept optional capture writers. In `watch`, wrap each with `io.MultiWriter` so the **console echo is preserved** *and* the bytes are captured:

```
cmd.Stdout = io.MultiWriter(os.Stdout, capStdout)
cmd.Stderr = io.MultiWriter(os.Stderr, capStderr)
cmd.Stdin  = os.Stdin                 // unchanged
```

Signal forwarding and exit-code mapping stay exactly as today (`runner.go:29-53`). When capture writers are nil (e.g. shipping disabled), behaviour is byte-identical to today.

- **stdout and stderr are captured separately** to preserve level: `stdout → INFO`, `stderr → ERROR` (matches the JUnit importer's `system-out`→INFO / `system-err`→ERROR convention). **No per-line level parsing** — robust and language-agnostic.
- Each captured stream is read by a line scanner that **stamps `time.Now()` per line at read time** (correct ordering/timestamps; the backend returns launch logs in chronological order keyed on timestamp). Lines from both scanners are appended to a single, **mutex-guarded** NDJSON temp file (`{ts, level, message}` per line) so memory stays bounded on large runs.

### 4.2 Ship

New API client method (mirrors the existing `postJSON` calls in `internal/api/api.go`):

```go
// POST /api/launches/{id}/logs  — gate canIngestLogsForLaunch accepts the sulu_* API token.
func (c *Client) AppendLaunchLogs(launchID int64, entries []LogEntry) error
```

`LogEntry` serialises to the backend's `CreateLogEventRequest` shape:

```json
{ "timestamp": "2026-06-20T22:01:44.123", "level": "INFO", "message": "...", "source": "suluctl-console" }
```

(`timestamp` = ISO-8601 local date-time, parsed by `LocalDateTime`; `level` ∈ TRACE/DEBUG/INFO/WARN/ERROR — we emit only INFO/ERROR; `source` = `"suluctl-console"`, distinct from `allure-log-attachment` / `junit-import-suite`.)

- `watch` already holds `session.LaunchID` (numeric `int64`) from `CreateLaunch` (`watch.go:88-104`, `api.go:28-29`) — the sink is addressable with zero extra calls.
- **Flush once, at child exit**, after the final file sweep and before/with `finishAndReport` (`watch.go:190-196`). Read the temp NDJSON back and POST in **bounded batches** (≈500 lines or ≈256 KB per request) — the V1 log endpoint has no documented payload cap, so the client batches defensively.

### 4.3 Default & kill-switch

- **ON by default.** No existing users to surprise; the goal is that `watch` "just ships logs."
- Kill-switch: `--ship-console=false` / env `SULU_SHIP_CONSOLE=false` disables capture (runner gets nil writers → today's exact passthrough).

### 4.4 Fail-safe (load-bearing — matches existing `watch` philosophy)

Log shipping is best-effort and **must never affect the wrapped command's exit code** (`watch.go:22-23` already guarantees "Sulu being unreachable only degrades streaming"):

- Sulu unreachable / 5xx / auth error → rate-limited WARNING to stderr (reuse the `failStreak` pattern, `watch.go:146-150`), drop the flush, return the child's exit code unchanged.
- Capture/temp-file errors → WARNING, continue running the command.

### 4.5 Known caveat (documented, not a bug)

Frameworks that capture stdout **in-process** (pytest default, Surefire `redirectTestOutputToFile`, Playwright HTML reporter) divert output away from the OS pipe → the tee sees little. That is expected; **O2 covers per-test logs** for exactly these stacks. Document in `--help` and README.

### 4.6 Adoption note for the reference suite

The reference suite currently invokes `suluctl upload` (`run/run-backend.sh:12`, `run/run-ui.sh:9`). O1 only applies to `watch`, so adopting it means switching the run scripts to `suluctl watch --results <dir> -- ./gradlew test`. This is an ops change in `aisulu-prod-tests`, out of scope for this repo's code change but called out so the value is reachable.

---

## 5. Track O2 — `init` scaffolds per-framework log→Allure glue (per-test)

### 5.1 How the scaffolder works today (context)

`internal/initscaffold/`:
- `Registry(kind)` → `Framework` struct: `ResultsDir`, `TestCmd`, `Manifest` mode (`patchPackageJSON` / `patchCsproj` / `printOnly`), `ManifestSnippet`, `ManualSteps` (`scaffold.go:34-79`).
- `Render(fw)` → walks `templates/<kind>/`, runs each `.tmpl` through Go `text/template` (injecting `.Package`), writes files with a managed-file stamp; **create-if-absent, skip-on-drift unless `--force`** (`render.go:25-88`).
- `PatchManifest(fw)` → patches `package.json`/`.csproj`; for Gradle/pytest it **prints** the snippet (`printOnly`) because Go has no robust Gradle/TOML parser (`patch.go:27-36`).

For TestNG today `Render` drops `SuluLabelListener.java` (whose `afterInvocation` is a **no-op** today, `templates/.../SuluLabelListener.java.tmpl:74-76`), the SPI registration, `@SuluTest`/`Priority`, and `Sulu.java`; `PatchManifest` prints "add allure-testng".

### 5.2 What O2 adds (TestNG + JUnit5 — the real work)

**(a) New template `templates/testng/src/test/java/__PKG__/SuluLogAppender.java.tmpl`** — a custom log4j2 `@Plugin` appender buffering log output **per thread** (parallel `@Test`s don't mix):

```java
package {{.Package}};

import io.qameta.allure.Allure;
import org.apache.logging.log4j.core.*;
import org.apache.logging.log4j.core.appender.AbstractAppender;
import org.apache.logging.log4j.core.config.plugins.*;
import org.apache.logging.log4j.core.layout.PatternLayout;
import java.io.Serializable;

@Plugin(name = "SuluLog", category = Core.CATEGORY_NAME, elementType = Appender.ELEMENT_TYPE)
public final class SuluLogAppender extends AbstractAppender {
    private static final ThreadLocal<StringBuilder> BUF = ThreadLocal.withInitial(StringBuilder::new);

    private SuluLogAppender(String name, Layout<? extends Serializable> layout) {
        super(name, null, layout, true, Property.EMPTY_ARRAY);
    }
    @PluginFactory
    public static SuluLogAppender create(@PluginAttribute("name") String name,
                                         @PluginElement("Layout") Layout<? extends Serializable> layout) {
        if (layout == null) layout = PatternLayout.newBuilder()
            .withPattern("%d{HH:mm:ss.SSS} %-5level %logger{1} - %msg%n").build();
        return new SuluLogAppender(name == null ? "SuluLog" : name, layout);
    }
    @Override public void append(LogEvent e) { BUF.get().append(new String(getLayout().toByteArray(e))); }

    /** per-test listener calls this: return + clear this thread's buffer */
    public static String drainCurrentThread() {
        StringBuilder sb = BUF.get(); String s = sb.toString(); sb.setLength(0); return s;
    }
}
```

**(b) Fill the existing no-op `afterInvocation`** in `SuluLabelListener.java.tmpl` — the per-test flush, fired while the Allure test context is still live (same ordering guarantee the listener's javadoc already documents):

```java
@Override
public void afterInvocation(final IInvokedMethod method, final ITestResult testResult) {
    if (!method.isTestMethod()) return;
    final String log = SuluLogAppender.drainCurrentThread();
    if (!log.isBlank()) {
        // contract with Sulu's importer (PR #152): name=log, MIME exactly text/plain, ext .txt
        Allure.addAttachment("log", "text/plain", log, ".txt");
    }
}
```

JUnit5 is the mirror: a `SuluLogAppender` + flush in `SuluAllureExtension`'s `afterEach` (it already exists as scaffolded glue).

**(c) `Registry()` change** (`scaffold.go`, TestNG/JUnit5 cases): add the `log4j-core` dependency to the printed snippet and an appender-registration `ManualSteps` entry:

```go
ManifestSnippet: "Add to build.gradle:\n" +
    "  testImplementation 'io.qameta.allure:allure-testng:2.34.0'\n" +
    "  testImplementation 'org.apache.logging.log4j:log4j-core:2.23.1'  // SuluLog appender\n" +
    "  test { systemProperty 'allure.results.directory', \"$buildDir/allure-results\" }",
ManualSteps: []string{
    "Register the appender in src/test/resources/log4j2.xml:\n" +
    "    <SuluLog name=\"SuluLog\"><PatternLayout pattern=\"%d{HH:mm:ss.SSS} %-5level %logger{1} - %msg%n\"/></SuluLog>\n" +
    "  then add <AppenderRef ref=\"SuluLog\"/> inside <Root>.",
},
```

### 5.3 `log4j2.xml` registration (honest mechanism)

The scaffolder **creates** files but cannot safely merge an arbitrary existing XML. So appender registration follows the **existing** un-mergeable-config pattern (`printOnly` for Gradle, `ManualSteps` for `playwright.config.ts`):

- **No `log4j2.xml` present** → scaffold a complete one (CONSOLE + `SuluLog`) as a template (`Render` "create" path).
- **`log4j2.xml` present** (e.g. the reference suite) → print the `<SuluLog/>` + `<AppenderRef/>` snippet as a `ManualSteps` entry for the user to paste.

No new merge mechanism is introduced.

### 5.4 pytest / Playwright — near-free

`allure-pytest` and `allure-playwright` already attach captured log/stdout/stderr as `text/plain` (byte-compatible with the PR #152 routing). The existing `Registry` entries already wire the plugin (`scaffold.go:48-66`). O2 here is documentation only: note that logs already flow and how to widen capture (`log_cli`, `--alluredir`). No new code.

### 5.5 xUnit — deferred (v0.4)

`SuluTestAttribute.cs` writes allure-results JSON directly (no console-host hook on v3). Log capture is scoped to the already-planned v0.4 work.

### 5.6 Cross-repo contract (must not drift)

The scaffolded glue emits exactly one per-test attachment: **name `log`, MIME `text/plain` (bare — NO `; charset=…`, which breaks the backend's exact-equals check), file `.txt`**, `PatternLayout` carrying a leading `HH:mm:ss.SSS` + level so Sulu's `LogAttachmentParser` reads time + level. These three constants are locked against the backend's `LogAttachmentParser.isLogAttachment` regex + MIME check.

---

## 6. Sequencing

1. **O1 first** (suluctl `watch`): smallest, universal, immediate "console in Sulu" win — one runner change + a line-scanner + `AppendLaunchLogs` + batched flush, default ON, zero backend change.
2. **O2 next** (suluctl `init`): start with the log4j2 appender (the reference suite's actual gap); pytest/Playwright are documentation; xUnit deferred to v0.4.

## 7. Test plan

- **O1 unit:** `runner.Run` tees to capture writers while preserving console echo + exit code + signal forwarding; nil writers ⇒ byte-identical passthrough. Line scanner stamps per line; stdout→INFO, stderr→ERROR. Batcher splits at the line/byte thresholds.
- **O1 contract:** `AppendLaunchLogs` against an `httptest` mock asserting the `POST /api/launches/{id}/logs` body shape; fail-safe paths (network error, 5xx, 409) never change the child exit code.
- **O1 live E2E:** `watch -- <cmd that prints to stdout/stderr>` against a local backend → assert launch-scoped rows appear via `GET /api/launches/{id}/logs`.
- **O2 render:** `Render(TestNG/JUnit5)` writes `SuluLogAppender.java` + the filled listener; `golden` test on the emitted Java; `Registry` snippet/ManualSteps include the log4j-core dep + the `<SuluLog/>` registration.
- **O2 live E2E:** scaffold into a throwaway log4j2 project, run a test that logs, assert a `log`/`text/plain` attachment in allure-results and (after `suluctl upload`) per-test rows via `GET /api/test-results/{id}/logs`.

## 8. Risks

- **Large runs (O1):** unbounded console → temp-file streaming + bounded batches mitigate; document a soft cap.
- **MIME drift (O2):** a `text/plain; charset=…` suffix silently breaks routing → the appender emits bare `text/plain`; covered by an E2E assertion.
- **In-process capture (O1):** sparse console for some frameworks — documented; O2 is the answer for per-test fidelity.
