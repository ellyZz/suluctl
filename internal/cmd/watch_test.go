package cmd

import (
	"bytes"
	"path/filepath"
	"runtime"
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
