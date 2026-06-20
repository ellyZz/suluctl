# suluctl log capture — Track O1 (watch console → launch-scoped logs) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `suluctl watch` tees the wrapped command's stdout/stderr and ships it to Sulu as launch-scoped logs, ON by default, so a run's console appears in the launch's Logs panel for any language/framework.

**Architecture:** A new `internal/console` package line-buffers and caps captured output in memory. `internal/runner` gains a capture-aware `RunWithCapture` (the existing `Run` stays a nil-capture wrapper, so nothing else breaks). `internal/api` gains `AppendLaunchLogs` (batched POST to the existing `/api/launches/{id}/logs`). `internal/cmd/watch.go` wires them: when `cfg.ShipConsole`, it tees the child through `io.MultiWriter(os.Stdout, capturer)` and flushes once, just before `finishAndReport`. Shipping is best-effort and never changes the child's exit code.

**Tech Stack:** Go 1.25, stdlib only (`net/http`, `bufio`/`bytes`, `io`, `time`, `sync`). Tests use `net/http/httptest` + the existing `mockSulu` harness; CI runs `gofmt`, `go vet`, `go test -race`.

**Refinement vs spec:** the spec §4.1/§8 mention a temp NDJSON file; this plan uses a **bounded in-memory accumulator** instead — with flush-once-at-exit a temp file adds cleanup/signal complexity for no benefit, and the 50 MB / 200k-line hard cap keeps memory safe. The spec's two temp-file lines are updated to match.

## Global Constraints

- **Module:** `github.com/ellyZz/suluctl`; **Go 1.25**; stdlib only (no new deps).
- **Backend contract (do not drift):** `POST /api/launches/{id}/logs`, body = a **JSON array** of `{timestamp, level, message, source}`; gate `canIngestLogsForLaunch` accepts the `sulu_*` API token. `message` is **`@NotBlank`**, `level`+`timestamp` are **`@NotNull`** — never send a blank message or a null field (it 400s the whole batch).
- **Timestamp format:** `"2006-01-02T15:04:05.000"` (ISO-8601 local, no offset — parses into Java `LocalDateTime`).
- **Level values:** only `"INFO"` (stdout) and `"ERROR"` (stderr) are emitted (both are valid `LogLevel` names).
- **Source tag:** `"suluctl-console"`.
- **Default ON**, kill-switch `--ship-console=false` / `SULU_SHIP_CONSOLE=false`.
- **Fail-safe (load-bearing):** console capture/shipping must **never** change the wrapped command's exit code; all failures are rate-limited WARNINGs to stderr.
- **Caps:** `maxLines = 200_000`, `maxBytes = 50 << 20` (50 MB); past either, set `truncated` and stop appending.
- **Batch size:** ≤ 500 entries per POST; a failed batch aborts the remainder (the endpoint is not idempotent — never re-send a sent batch).
- **Every task ends green:** `gofmt -l` clean, `go vet ./...` clean, `go test ./... -race` passing.

---

### Task 1: Config — `SULU_SHIP_CONSOLE` (default ON)

**Files:**
- Modify: `internal/config/config.go` (add `ShipConsole` field; read env in `FromEnv`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config.ShipConsole bool` (default `true`; `SULU_SHIP_CONSOLE` in {`false`,`0`,`FALSE`,…} → `false`).

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestFromEnvShipConsoleDefaultsTrue(t *testing.T) {
	t.Setenv("SULU_SHIP_CONSOLE", "") // present-but-empty == unset for our parser
	if !FromEnv().ShipConsole {
		t.Error("SULU_SHIP_CONSOLE unset must default to true")
	}
}

func TestFromEnvShipConsoleCanDisable(t *testing.T) {
	for _, v := range []string{"false", "0", "FALSE"} {
		t.Setenv("SULU_SHIP_CONSOLE", v)
		if FromEnv().ShipConsole {
			t.Errorf("SULU_SHIP_CONSOLE=%q must disable shipping", v)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ruslan/projects/aisulu_dev/suluctl && go test ./internal/config/ -run ShipConsole -v`
Expected: FAIL — `ShipConsole` is an unknown field (compile error).

- [ ] **Step 3: Add the field + env read**

In `internal/config/config.go`, add the field to the `Config` struct (after `Insecure bool`):

```go
	Insecure    bool
	ShipConsole bool
```

In `FromEnv`, just before `return cfg`:

```go
	cfg.ShipConsole = true
	if v := os.Getenv("SULU_SHIP_CONSOLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.ShipConsole = b
		}
	}
```

(`strconv` and `os` are already imported.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run ShipConsole -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): SULU_SHIP_CONSOLE flag (default on)"
```

---

### Task 2: `internal/console` — bounded line capture

**Files:**
- Create: `internal/console/console.go`
- Test: `internal/console/console_test.go`

**Interfaces:**
- Produces:
  - `console.New() *Capturer`
  - `(*Capturer).Writer(level string) *LineWriter` — an `io.Writer`; tee it: `io.MultiWriter(os.Stdout, c.Writer("INFO"))`
  - `(*LineWriter).Flush()` — emit the trailing partial line (call once, after the child exits)
  - `(*Capturer).Entries() []console.Entry` — snapshot `{TS time.Time, Level, Message string}`
  - `(*Capturer).Truncated() bool`
  - exported consts `MaxLines = 200_000`, `MaxBytes = 50 << 20`
  - field `(*Capturer).now func() time.Time` (injectable for tests)

- [ ] **Step 1: Write the failing tests**

Create `internal/console/console_test.go`:

```go
package console

import (
	"testing"
	"time"
)

func TestLineBufferingBlankFilterAndPartialFlush(t *testing.T) {
	c := New()
	fixed := time.Date(2026, 6, 20, 22, 1, 44, 0, time.UTC)
	c.now = func() time.Time { return fixed }
	w := c.Writer("INFO")
	w.Write([]byte("hello "))        // partial line
	w.Write([]byte("world\n\n  \n")) // completes line 1, then two blank lines (dropped)
	w.Write([]byte("tail-no-nl"))    // partial, no newline yet
	w.Flush()                        // emits the trailing partial

	got := c.Entries()
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].Message != "hello world" || got[0].Level != "INFO" || !got[0].TS.Equal(fixed) {
		t.Errorf("entry0 = %+v", got[0])
	}
	if got[1].Message != "tail-no-nl" {
		t.Errorf("entry1 = %+v", got[1])
	}
}

func TestStripsTrailingCR(t *testing.T) {
	c := New()
	c.Writer("ERROR").Write([]byte("win line\r\n"))
	got := c.Entries()
	if len(got) != 1 || got[0].Message != "win line" || got[0].Level != "ERROR" {
		t.Errorf("got %+v", got)
	}
}

func TestCapTruncates(t *testing.T) {
	c := New()
	w := c.Writer("INFO")
	for i := 0; i < MaxLines+10; i++ {
		w.Write([]byte("x\n"))
	}
	if !c.Truncated() {
		t.Error("must mark truncated past the line cap")
	}
	if n := len(c.Entries()); n != MaxLines {
		t.Errorf("must cap at %d entries, got %d", MaxLines, n)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/console/ -v`
Expected: FAIL — package `console` does not exist.

- [ ] **Step 3: Implement the package**

Create `internal/console/console.go`:

```go
// Package console captures a wrapped command's stdout/stderr line-by-line so
// `suluctl watch` can ship it to Sulu as launch-scoped logs. Bounded in memory.
package console

import (
	"bytes"
	"strings"
	"sync"
	"time"
)

// Caps: past either, capture stops and Truncated() reports true. The flush-once
// -at-exit model makes an on-disk spill unnecessary at this size.
const (
	MaxLines = 200_000
	MaxBytes = 50 << 20 // 50 MB
)

// Entry is one captured, non-blank line.
type Entry struct {
	TS      time.Time
	Level   string // "INFO" (stdout) | "ERROR" (stderr)
	Message string
}

// Capturer accumulates lines from one or more LineWriters, bounded by the caps.
type Capturer struct {
	mu        sync.Mutex
	entries   []Entry
	bytes     int
	truncated bool
	now       func() time.Time
}

func New() *Capturer { return &Capturer{now: time.Now} }

// Writer returns an io.Writer recording complete, non-blank lines at the given level.
func (c *Capturer) Writer(level string) *LineWriter { return &LineWriter{cap: c, level: level} }

func (c *Capturer) add(level, msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.truncated || len(c.entries) >= MaxLines || c.bytes+len(msg) > MaxBytes {
		c.truncated = true
		return
	}
	c.entries = append(c.entries, Entry{TS: c.now(), Level: level, Message: msg})
	c.bytes += len(msg)
}

// Entries returns a snapshot. Call after flushing every LineWriter.
func (c *Capturer) Entries() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Entry, len(c.entries))
	copy(out, c.entries)
	return out
}

// Truncated reports whether the cap was hit (some output dropped).
func (c *Capturer) Truncated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.truncated
}

// LineWriter is an io.Writer emitting one Entry per complete line.
type LineWriter struct {
	cap   *Capturer
	level string
	mu    sync.Mutex
	buf   []byte
}

func (w *LineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// Flush emits any trailing partial line (no newline at EOF). Call once after the child exits.
func (w *LineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(w.buf)
		w.buf = nil
	}
}

func (w *LineWriter) emit(line []byte) {
	s := strings.TrimRight(string(line), "\r")
	if strings.TrimSpace(s) == "" {
		return // backend message is @NotBlank — never record empty lines
	}
	w.cap.add(w.level, s)
}
```

- [ ] **Step 4: Run tests + race + vet to verify they pass**

Run: `go test ./internal/console/ -race -v && go vet ./internal/console/`
Expected: PASS, no vet output.

- [ ] **Step 5: Commit**

```bash
git add internal/console/
git commit -m "feat(console): bounded line capturer for stdout/stderr"
```

---

### Task 3: `internal/api` — `AppendLaunchLogs` (batched)

**Files:**
- Modify: `internal/api/api.go` (add `LogEntry` type + `AppendLaunchLogs` + `logBatchSize`)
- Test: `internal/api/api_test.go`

**Interfaces:**
- Consumes: existing `(*Client).postJSON`, `(*Client).withRetry`, `*APIError`.
- Produces:
  - `api.LogEntry{Timestamp, Level, Message, Source string}` (JSON: `timestamp`/`level`/`message`/`source`)
  - `(*Client).AppendLaunchLogs(launchID int64, entries []LogEntry) error`

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/api_test.go`:

```go
func TestAppendLaunchLogsBatches(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var total int
	var first LogEntry
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch []LogEntry
		_ = json.NewDecoder(r.Body).Decode(&batch)
		mu.Lock()
		paths = append(paths, r.URL.Path)
		if total == 0 && len(batch) > 0 {
			first = batch[0]
		}
		total += len(batch)
		mu.Unlock()
	}))
	entries := make([]LogEntry, 1100) // > two batches of 500
	for i := range entries {
		entries[i] = LogEntry{Timestamp: "2026-06-20T22:01:44.000", Level: "INFO", Message: "m", Source: "suluctl-console"}
	}
	if err := c.AppendLaunchLogs(42, entries); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if total != 1100 {
		t.Errorf("want 1100 entries delivered, got %d", total)
	}
	if len(paths) != 3 { // 500 + 500 + 100
		t.Errorf("want 3 batched POSTs, got %d (%v)", len(paths), paths)
	}
	if paths[0] != "/api/launches/42/logs" {
		t.Errorf("path = %q", paths[0])
	}
	if first.Source != "suluctl-console" || first.Level != "INFO" {
		t.Errorf("bad entry shape: %+v", first)
	}
}

func TestAppendLaunchLogsAbortsOnBatchError(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest) // 400 = non-retryable -> abort, no re-send
	}))
	entries := make([]LogEntry, 600) // two batches
	for i := range entries {
		entries[i] = LogEntry{Timestamp: "2026-06-20T22:01:44.000", Level: "INFO", Message: "m"}
	}
	if err := c.AppendLaunchLogs(42, entries); err == nil {
		t.Fatal("want error from the failed first batch")
	}
	if calls.Load() != 1 {
		t.Errorf("must abort after the first failed batch, got %d calls", calls.Load())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run AppendLaunchLogs -v`
Expected: FAIL — `LogEntry` / `AppendLaunchLogs` undefined (compile error).

- [ ] **Step 3: Implement**

Add to `internal/api/api.go` (e.g. just after the `Finish` method):

```go
// LogEntry maps to the backend CreateLogEventRequest. Timestamp is ISO-8601 local
// (no offset) so it parses into Java LocalDateTime; Level is a LogLevel name.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source"`
}

const logBatchSize = 500

// AppendLaunchLogs posts launch-scoped log events to POST /api/launches/{id}/logs
// in batches of logBatchSize. Single pass: a failed batch aborts the remainder
// (the endpoint is not idempotent — a re-sent batch would duplicate).
func (c *Client) AppendLaunchLogs(launchID int64, entries []LogEntry) error {
	path := fmt.Sprintf("/api/launches/%d/logs", launchID)
	for start := 0; start < len(entries); start += logBatchSize {
		end := start + logBatchSize
		if end > len(entries) {
			end = len(entries)
		}
		if err := c.postJSON(path, entries[start:end], nil); err != nil {
			return err
		}
	}
	return nil
}
```

(`fmt` is already imported in `api.go`.)

- [ ] **Step 4: Run tests + race to verify they pass**

Run: `go test ./internal/api/ -race -v`
Expected: PASS (the new tests + all existing).

- [ ] **Step 5: Commit**

```bash
git add internal/api/api.go internal/api/api_test.go
git commit -m "feat(api): AppendLaunchLogs — batched POST /api/launches/{id}/logs"
```

---

### Task 4: `internal/runner` — capture-aware `RunWithCapture`

**Files:**
- Modify: `internal/runner/runner.go` (add `RunWithCapture`; `Run` becomes a nil-capture wrapper)
- Test: `internal/runner/runner_test.go` (add a capture test; existing tests unchanged)

**Interfaces:**
- Consumes: nothing new.
- Produces: `runner.RunWithCapture(argv []string, stdout, stderr io.Writer) (code int, interrupted bool, err error)` — nil writer inherits the parent's `os.Stdout`/`os.Stderr`; stdin always inherited. `runner.Run(argv)` is unchanged in behaviour (delegates with nil, nil).

- [ ] **Step 1: Write the failing test**

Add to `internal/runner/runner_test.go` (extend the import block with `"bytes"` and `"strings"`):

```go
func TestRunWithCaptureTeesStdoutAndStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	var buf bytes.Buffer
	code, interrupted, err := RunWithCapture(
		[]string{"sh", "-c", "echo hello; echo oops 1>&2"}, &buf, &buf)
	if err != nil || interrupted || code != 0 {
		t.Fatalf("code=%d interrupted=%v err=%v", code, interrupted, err)
	}
	s := buf.String()
	if !strings.Contains(s, "hello") || !strings.Contains(s, "oops") {
		t.Errorf("capture missing child output: %q", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runner/ -run RunWithCapture -v`
Expected: FAIL — `RunWithCapture` undefined (compile error).

- [ ] **Step 3: Implement**

In `internal/runner/runner.go`, add `"io"` to the import block, then replace the `Run` function with:

```go
// Run is RunWithCapture with stdout/stderr inherited from the parent (no capture).
func Run(argv []string) (code int, interrupted bool, err error) {
	return RunWithCapture(argv, nil, nil)
}

// RunWithCapture starts argv, forwards SIGINT/SIGTERM to the child, and blocks until
// it exits. stdout/stderr, when non-nil, receive the child's streams — pass
// io.MultiWriter(os.Stdout, capture) to both echo and capture; nil inherits the
// parent's os.Stdout/os.Stderr. stdin is always inherited. Returns the child's exit
// code and whether a signal arrived; err is non-nil only when the child could not be
// started (code 127). A signal-killed child yields ExitCode() == -1.
func RunWithCapture(argv []string, stdout, stderr io.Writer) (code int, interrupted bool, err error) {
	if len(argv) == 0 {
		return 127, false, errors.New("empty command")
	}
	out := stdout
	if out == nil {
		out = os.Stdout
	}
	errW := stderr
	if errW == nil {
		errW = os.Stderr
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, errW

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		return 127, false, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case sig := <-sigCh:
			interrupted = true
			_ = cmd.Process.Signal(sig)
		case waitErr := <-done:
			if waitErr == nil {
				return 0, interrupted, nil
			}
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				return exitErr.ExitCode(), interrupted, nil
			}
			return 127, interrupted, waitErr
		}
	}
}
```

(The body from `sigCh` onward is identical to today's `Run` — only the signature, the `out`/`errW` defaulting, and the `cmd.Stdout/Stderr` assignment changed. The package doc comment at the top of the old `Run` can stay or move onto `RunWithCapture`.)

- [ ] **Step 4: Run the whole runner suite + race to verify all pass**

Run: `go test ./internal/runner/ -race -v`
Expected: PASS — the new capture test plus the unchanged `TestExitCodePassthrough` / `TestStartFailure` / `TestEmptyArgv`.

- [ ] **Step 5: Commit**

```bash
git add internal/runner/runner.go internal/runner/runner_test.go
git commit -m "feat(runner): RunWithCapture — optional stdout/stderr tee"
```

---

### Task 5: `internal/cmd/watch` — wire capture, flush, flag, fail-safe

**Files:**
- Modify: `internal/cmd/watch.go` (flag, capturer, `RunWithCapture`, flush+ship before `finishAndReport`, helpers)
- Modify: `internal/cmd/mockserver_test.go` (record posted logs)
- Test: `internal/cmd/watch_test.go`

**Interfaces:**
- Consumes: `config.Config.ShipConsole` (Task 1), `console.New/Writer/Flush/Entries/Truncated` (Task 2), `api.LogEntry`/`AppendLaunchLogs` (Task 3), `runner.RunWithCapture` (Task 4), existing `newClient`, `finishAndReport`, `exitResult`.
- Produces: `--ship-console` flag; launch-scoped console logs posted with `source="suluctl-console"` on a successful launch.

- [ ] **Step 1: Extend the mock server to record logs**

In `internal/cmd/mockserver_test.go`, add a field to `mockSulu`:

```go
	uploads       int
	logs          []map[string]any
```

and register the handler inside `newMockSulu` (next to the other routes):

```go
	mux.HandleFunc("POST /api/launches/42/logs", func(w http.ResponseWriter, r *http.Request) {
		var batch []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&batch)
		m.mu.Lock()
		m.logs = append(m.logs, batch...)
		m.mu.Unlock()
	})
```

- [ ] **Step 2: Write the failing tests**

Add to `internal/cmd/watch_test.go` (add `"strings"` to its import block):

```go
func TestWatchShipsConsoleLogs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	neutralizeEnv(t)
	old := watchTick
	watchTick = 50 * time.Millisecond
	t.Cleanup(func() { watchTick = old })

	m := newMockSulu(t)
	dir := filepath.Join(t.TempDir(), "results")

	var out, errB bytes.Buffer
	code := Watch([]string{
		"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1", "--",
		"sh", "-c", `echo console-line-1 && echo err-line 1>&2 && exit 0`,
	}, &out, &errB, "test")
	if code != 0 {
		t.Fatalf("exit %d (stderr: %s)", code, errB.String())
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var msgs, levels []string
	for _, e := range m.logs {
		msgs = append(msgs, e["message"].(string))
		levels = append(levels, e["level"].(string))
	}
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "console-line-1") {
		t.Errorf("stdout line not shipped: %v", msgs)
	}
	gotErr := false
	for i, msg := range msgs {
		if msg == "err-line" && levels[i] == "ERROR" {
			gotErr = true
		}
	}
	if !gotErr {
		t.Errorf("stderr line must ship at ERROR: msgs=%v levels=%v", msgs, levels)
	}
	for _, e := range m.logs {
		if e["source"] != "suluctl-console" {
			t.Errorf("source must be suluctl-console, got %v", e["source"])
		}
	}
}

func TestWatchShipConsoleDisabledShipsNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	neutralizeEnv(t)
	old := watchTick
	watchTick = 50 * time.Millisecond
	t.Cleanup(func() { watchTick = old })

	m := newMockSulu(t)
	dir := filepath.Join(t.TempDir(), "results")

	var out, errB bytes.Buffer
	code := Watch([]string{
		"--results", dir, "--ship-console=false", "--url", m.srv.URL, "--token", "t", "--project", "1", "--",
		"sh", "-c", `echo nope && exit 0`,
	}, &out, &errB, "test")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.logs) != 0 {
		t.Errorf("--ship-console=false must ship nothing, got %v", m.logs)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run 'WatchShips|WatchShipConsole' -v`
Expected: FAIL — `--ship-console` is an unknown flag (usage error / no logs recorded).

- [ ] **Step 4: Wire watch.go**

In `internal/cmd/watch.go`:

(a) Add to the import block: `"io"` and `"github.com/ellyZz/suluctl/internal/console"`.

(b) Register the flag inside the `flag.NewFlagSet("watch", …)` block (next to the other `fs.*Var` calls):

```go
	fs.BoolVar(&cfg.ShipConsole, "ship-console", cfg.ShipConsole,
		"ship the wrapped command's console output to Sulu as launch-scoped logs (env SULU_SHIP_CONSOLE; default true)")
```

(c) Replace the child-launch goroutine (currently `go func() { code, intr, err := runner.Run(childArgv); … }()`) with capture wiring:

```go
	var capr *console.Capturer
	var capOut, capErr *console.LineWriter
	var childOut, childErr io.Writer // nil => runner inherits os.Stdout/os.Stderr
	if cfg.ShipConsole {
		capr = console.New()
		capOut = capr.Writer("INFO")
		capErr = capr.Writer("ERROR")
		childOut = io.MultiWriter(os.Stdout, capOut)
		childErr = io.MultiWriter(os.Stderr, capErr)
	}
	go func() {
		code, intr, err := runner.RunWithCapture(childArgv, childOut, childErr)
		exitCh <- exitResult{code, intr, err}
	}()
```

(d) After the watch loop, before the `finishAndReport(...)` call, flush + ship:

```go
	if cfg.ShipConsole && capr != nil {
		capOut.Flush()
		capErr.Flush()
		shipConsole(client, session.LaunchID, capr, errW)
	}
```

(e) Add the two helpers at the bottom of `watch.go`:

```go
// shipConsole best-effort posts the captured console as launch-scoped logs.
// Failures are warnings only — they never affect the wrapped command's exit code.
func shipConsole(client *api.Client, launchID int64, capr *console.Capturer, errW io.Writer) {
	if capr.Truncated() {
		fmt.Fprintln(errW, "WARNING: console output exceeded the capture cap — shipped logs are truncated")
	}
	entries := toLogEntries(capr.Entries())
	if len(entries) == 0 {
		return
	}
	if err := client.AppendLaunchLogs(launchID, entries); err != nil {
		fmt.Fprintf(errW, "WARNING: shipping console logs failed (%v)\n", err)
	}
}

func toLogEntries(in []console.Entry) []api.LogEntry {
	out := make([]api.LogEntry, len(in))
	for i, e := range in {
		out[i] = api.LogEntry{
			Timestamp: e.TS.Format("2006-01-02T15:04:05.000"),
			Level:     e.Level,
			Message:   e.Message,
			Source:    "suluctl-console",
		}
	}
	return out
}
```

(`api`, `fmt`, `os` are already imported in `watch.go`; add `io` and `console` per (a).)

- [ ] **Step 5: Run the full cmd suite + race to verify all pass**

Run: `go test ./internal/cmd/ -race -v`
Expected: PASS — the two new tests plus every existing watch/upload/report test (exit-code passthrough, degraded-wrapper, 409, interrupt-STOPPED, etc. are unaffected because shipping is best-effort and additive).

- [ ] **Step 6: Commit**

```bash
git add internal/cmd/watch.go internal/cmd/mockserver_test.go internal/cmd/watch_test.go
git commit -m "feat(watch): ship console to launch-scoped logs (default on, --ship-console kill-switch)"
```

---

### Task 6: Docs — README + security/PII callout

**Files:**
- Modify: `README.md`

**Interfaces:** none (documentation).

- [ ] **Step 1: Add a "Console logs" subsection to README.md**

Under the `watch` documentation, add:

```markdown
### Console logs (launch-scoped)

`suluctl watch` ships the wrapped command's stdout/stderr to Sulu as
**launch-scoped logs** — they appear in the launch's Logs panel. This is
**on by default** and works for any language/framework, since suluctl just
tees the console of whatever you run.

- Disable with `--ship-console=false` or `SULU_SHIP_CONSOLE=false`.
- stdout lines are recorded at `INFO`, stderr at `ERROR`.
- Capture is best-effort: if Sulu is unreachable, your test command's exit
  code is never affected.
- Capped at ~50 MB / 200k lines per run (beyond that, logs are truncated with
  a warning).

> ⚠️ **Security:** console output is sent to Sulu as-is. Do not print secrets,
> tokens, or PII in tests — or set `SULU_SHIP_CONSOLE=false`.

> ℹ️ This is **launch-scoped** (the whole run's console). For **per-test** logs
> in each result's Logs panel, use your framework's Allure log integration
> (see `suluctl init`).
```

(Place it so it reads naturally after the existing `watch` description; match the surrounding heading level.)

- [ ] **Step 2: Verify the build + full suite once more**

Run: `gofmt -l . && go vet ./... && go test ./... -race`
Expected: no `gofmt` output, no `vet` output, all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document watch console-log shipping + SULU_SHIP_CONSOLE"
```

---

## Self-Review

**Spec coverage (§ → task):**
- §4.1 capture (tee, separate stdout/stderr levels, per-line stamp, blank-line drop, CR strip, caps) → Task 2 (console) + Task 4 (runner tee) + Task 5 (wiring).
- §4.2 ship (existing endpoint, batch, flush strictly before Finish, single-pass no-retry) → Task 3 (`AppendLaunchLogs` batch/abort) + Task 5 (flush before `finishAndReport`).
- §4.3 default ON + kill-switch + `SULU_SHIP_CONSOLE` in `FromEnv` + PII callout → Task 1 + Task 5 (flag) + Task 6 (callout).
- §4.4 fail-safe (never change exit code) → Task 5 `shipConsole` warns only; covered by existing exit-code tests staying green.
- §7 test plan (runner tee, contract/batch, blank-line filter, live-ish watch E2E via mock) → Tasks 2–5.
- §8 risks (cap + truncation warning, double-surfacing/in-process caveats are documented in spec/README) → Task 2 cap + Task 5 truncation warning + Task 6 doc.
- Adoption note (§4.6, switch reference suite from `upload` to `watch`) is an ops step in `aisulu-prod-tests`, **out of scope** for this repo plan — flagged, not a task here.

**Placeholder scan:** none — every code/test step carries full code and an exact run command with expected output.

**Type consistency:** `console.Entry{TS,Level,Message}` (Task 2) → mapped by `toLogEntries` (Task 5) into `api.LogEntry{Timestamp,Level,Message,Source}` (Task 3); `RunWithCapture(argv, stdout, stderr io.Writer)` (Task 4) called with `io.MultiWriter`s in Task 5; `config.Config.ShipConsole` (Task 1) read by the Task 5 flag. `MaxLines`/`MaxBytes` exported in Task 2, used by the Task 2 cap test. Consistent across tasks.
