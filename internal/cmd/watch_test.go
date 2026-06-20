package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ellyZz/suluctl/internal/api"
	"github.com/ellyZz/suluctl/internal/config"
)

// api and config are needed by the newClient overrides below — per-file imports
// are required even within the same package.

// watchNoBackoff disables retry backoff sleeps for outage tests.
func watchNoBackoff(t *testing.T) {
	t.Helper()
	oldNew := newClient
	newClient = func(cfg config.Config, version string) *api.Client {
		c := oldNew(cfg, version)
		c.Sleep = func(time.Duration) {}
		return c
	}
	t.Cleanup(func() { newClient = oldNew })
}

func TestWatchStreamsAndPassesExitCode(t *testing.T) {
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
		"sh", "-c", `mkdir -p ` + dir + ` && echo '{"uuid":"w1"}' > ` + dir + `/w1-result.json && sleep 1 && exit 7`,
	}, &out, &errB, "test")

	if code != 7 {
		t.Fatalf("want child's exit code 7, got %d (stderr: %s)", code, errB.String())
	}
	m.mu.Lock()
	uploads, finished, finishReq := m.uploads, m.finished, m.finishReq
	m.mu.Unlock()
	if uploads < 1 {
		t.Error("no files were streamed")
	}
	if !finished {
		t.Error("finish was not called")
	}
	if finishReq["executionState"] != "" {
		t.Errorf("normal exit must finish FINISHED (empty body), got %v", finishReq)
	}
	if !hasLine(out.String(), "/app/launches/42") {
		t.Errorf("launch link missing:\n%s", out.String())
	}
}

func TestWatchDegradesToWrapperWhenSuluDown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	neutralizeEnv(t)
	watchNoBackoff(t)
	var out, errB bytes.Buffer
	// port 1 — nothing listens; CreateLaunch fails after retries
	code := Watch([]string{
		"--results", t.TempDir(), "--url", "http://127.0.0.1:1", "--token", "t", "--project", "1", "--",
		"sh", "-c", "exit 5",
	}, &out, &errB, "test")
	if code != 5 {
		t.Fatalf("degraded watch must pass through exit code 5, got %d", code)
	}
	if !hasLine(errB.String(), "WARNING") {
		t.Errorf("must warn about missing Sulu:\n%s", errB.String())
	}
}

func TestWatchRequiresChildCommand(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	if code := Watch([]string{"--results", t.TempDir()}, &out, &errB, "test"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestWatchSurvivesTransientOutage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	neutralizeEnv(t)
	old := watchTick
	watchTick = 50 * time.Millisecond
	t.Cleanup(func() { watchTick = old })
	watchNoBackoff(t)

	m := newMockSulu(t)
	m.failFiles = 5 // first uploadReady exhausts 4 attempts; a later tick succeeds
	dir := filepath.Join(t.TempDir(), "results")

	var out, errB bytes.Buffer
	code := Watch([]string{
		"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1", "--",
		"sh", "-c", `mkdir -p ` + dir + ` && echo '{"uuid":"t1"}' > ` + dir + `/t1-result.json && sleep 1 && exit 0`,
	}, &out, &errB, "test")
	if code != 0 {
		t.Fatalf("exit %d (stderr: %s)", code, errB.String())
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.ledger) != 1 {
		t.Fatalf("file must be uploaded after the outage clears, ledger: %v", m.ledger)
	}
}

func TestWatchCleanEmptiesResultsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	neutralizeEnv(t)
	old := watchTick
	watchTick = 50 * time.Millisecond
	t.Cleanup(func() { watchTick = old })

	m := newMockSulu(t)
	dir := t.TempDir()
	writeFixture(t, dir, "stale-result.json", `{"uuid":"stale"}`)

	var out, errB bytes.Buffer
	code := Watch([]string{
		"--results", dir, "--clean", "--url", m.srv.URL, "--token", "t", "--project", "1", "--",
		"sh", "-c", "exit 0",
	}, &out, &errB, "test")
	if code != 0 {
		t.Fatalf("exit %d (stderr: %s)", code, errB.String())
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.ledger {
		if row["fileName"] == "stale-result.json" {
			t.Error("--clean must remove stale results before the run")
		}
	}
}

func TestCleanDirRefusesRootAndHome(t *testing.T) {
	if err := cleanDir("/"); err == nil {
		t.Error("must refuse filesystem root")
	}
	if home, err := os.UserHomeDir(); err == nil {
		if err := cleanDir(home); err == nil {
			t.Error("must refuse home directory")
		}
	}
}

func TestWatchHelpLikeFlagValueIsUsageErrorNotPanic(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	code := Watch([]string{"--results", t.TempDir(), "--launch-name", "--help"}, &out, &errB, "test")
	if code != 2 {
		t.Fatalf("want usage error 2, got %d", code)
	}
}

func TestWatchHelpExitsZero(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	if code := Watch([]string{"-h"}, &out, &errB, "test"); code != 0 {
		t.Fatalf("-h must exit 0, got %d", code)
	}
	if !hasLine(errB.String(), "results") {
		t.Errorf("flag table must be printed:\n%s", errB.String())
	}
}

func TestWatch409StopsStreamingButStillFinishes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	neutralizeEnv(t)
	old := watchTick
	watchTick = 50 * time.Millisecond
	t.Cleanup(func() { watchTick = old })

	m := newMockSulu(t)
	m.finished = true // every /files answers 409 — session closed under our feet
	dir := filepath.Join(t.TempDir(), "results")

	var out, errB bytes.Buffer
	code := Watch([]string{
		"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1", "--",
		"sh", "-c", `mkdir -p ` + dir + ` && echo '{"uuid":"x"}' > ` + dir + `/x-result.json && sleep 1 && exit 0`,
	}, &out, &errB, "test")
	if code != 0 {
		t.Fatalf("child exit code must pass through, got %d (stderr: %s)", code, errB.String())
	}
	if !hasLine(errB.String(), "session is closed") {
		t.Errorf("must warn once about the closed session:\n%s", errB.String())
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.uploads != 0 {
		t.Errorf("no uploads may land on a closed session, got %d", m.uploads)
	}
}

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
	tsRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}$`)
	for _, e := range m.logs {
		ts, _ := e["timestamp"].(string)
		if !tsRe.MatchString(ts) {
			t.Errorf("timestamp must be LocalDateTime-shaped (no offset), got %q", ts)
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

func TestWatchInterruptFinishesStopped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signals")
	}
	neutralizeEnv(t)
	old := watchTick
	watchTick = 50 * time.Millisecond
	t.Cleanup(func() { watchTick = old })

	m := newMockSulu(t)
	dir := filepath.Join(t.TempDir(), "results")

	// SIGINT ourselves shortly after the child starts; runner.Run has Notify
	// active, forwards to the child (sh dies on it), and reports interrupted.
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	var out, errB bytes.Buffer
	code := Watch([]string{
		"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1", "--",
		"sh", "-c", "sleep 30",
	}, &out, &errB, "test")

	m.mu.Lock()
	state := m.finishReq["executionState"]
	m.mu.Unlock()
	if state != "STOPPED" {
		t.Errorf("interrupted watch must finish STOPPED, got %q (stderr: %s)", state, errB.String())
	}
	if code != 130 {
		t.Errorf("signal-killed child must map to 130, got %d", code)
	}
}
