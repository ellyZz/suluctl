package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ellyZz/suluctl/internal/api"
	"github.com/ellyZz/suluctl/internal/config"
)

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUploadHappyPath(t *testing.T) {
	neutralizeEnv(t)
	m := newMockSulu(t)
	dir := t.TempDir()
	writeFixture(t, dir, "a-result.json", `{"uuid":"a"}`)
	writeFixture(t, dir, "b-result.json", `{"uuid":"b"}`)

	var out, errB bytes.Buffer
	code := Upload([]string{
		"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1",
		"--launch-name", "ci run", "--tag", "smoke", "--env-var", "BRANCH=main",
	}, &out, &errB, "test")

	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errB.String())
	}
	m.mu.Lock()
	finished := m.finished
	m.mu.Unlock()
	if !finished {
		t.Error("finish was not called")
	}
	if !hasLine(out.String(), "parsed     2") {
		t.Errorf("summary missing:\n%s", out.String())
	}
	if !hasLine(out.String(), "/app/launches/42") {
		t.Errorf("launch link missing:\n%s", out.String())
	}
}

func TestFlagsOverrideEnv(t *testing.T) {
	neutralizeEnv(t)
	t.Setenv("SULU_LAUNCH_NAME", "from-env")
	m := newMockSulu(t)
	dir := t.TempDir()
	writeFixture(t, dir, "a-result.json", "{}")
	var out, errB bytes.Buffer
	code := Upload([]string{"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1",
		"--launch-name", "from-flag"}, &out, &errB, "test")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errB.String())
	}
	m.mu.Lock()
	name := m.createReq["name"]
	m.mu.Unlock()
	if name != "from-flag" {
		t.Errorf("flag must override env (spec §4), got %v", name)
	}
}

func TestUpload409IsTerminal(t *testing.T) {
	neutralizeEnv(t)
	m := newMockSulu(t)
	m.finished = true // session already finished server-side → /files answers 409
	dir := t.TempDir()
	writeFixture(t, dir, "a-result.json", "{}")
	writeFixture(t, dir, "b-result.json", "{}")
	var out, errB bytes.Buffer
	code := Upload([]string{"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1"}, &out, &errB, "test")
	if code != 1 {
		t.Fatalf("409 must be a total failure, got %d", code)
	}
	if !hasLine(errB.String(), "IMPORT_LAUNCH_FINISHED") {
		t.Errorf("must surface the conflict verbatim:\n%s", errB.String())
	}
	m.mu.Lock()
	uploads := m.uploads
	m.mu.Unlock()
	if uploads != 0 {
		t.Errorf("no further uploads may land after 409, got %d", uploads)
	}
}

func TestUploadMissingConfigIsUsageError(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	code := Upload([]string{"--results", t.TempDir()}, &out, &errB, "test")
	if code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if !hasLine(errB.String(), "SULU_TOKEN") {
		t.Errorf("error must mention missing config:\n%s", errB.String())
	}
}

func TestUpload402IsTotalFailure(t *testing.T) {
	neutralizeEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		// real 402 guard shape: human text under "error", no "message" key
		_, _ = w.Write([]byte(`{"error":"Workspace is in read-only mode","code":"WORKSPACE_READ_ONLY","upgradeUrl":"/app/billing","path":"/api/import/launches"}`))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	writeFixture(t, dir, "a-result.json", "{}")
	var out, errB bytes.Buffer
	code := Upload([]string{"--results", dir, "--url", srv.URL, "--token", "t", "--project", "1"}, &out, &errB, "test")
	if code != 1 {
		t.Fatalf("want 1, got %d", code)
	}
	if !hasLine(errB.String(), "Workspace is in read-only mode") {
		t.Errorf("server message must be verbatim:\n%s", errB.String())
	}
}

// uploadNoBackoff runs Upload with retry backoff sleeps disabled via the
// newClient seam (4 attempts × 1+2+4s of real sleeping is too slow for tests).
func uploadNoBackoff(t *testing.T, args []string, out, errB *bytes.Buffer) int {
	t.Helper()
	oldNew := newClient
	newClient = func(cfg config.Config, version string) *api.Client {
		c := oldNew(cfg, version)
		c.Sleep = func(time.Duration) {}
		return c
	}
	t.Cleanup(func() { newClient = oldNew })
	return Upload(args, out, errB, "test")
}

func TestUploadSingleFile413IsIsolated(t *testing.T) {
	neutralizeEnv(t)
	m := newMockSulu(t)
	m.reject413Next = 1
	dir := t.TempDir()
	writeFixture(t, dir, "a-result.json", "{}")
	var out, errB bytes.Buffer
	code := Upload([]string{"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1"}, &out, &errB, "test")
	if code != 0 {
		t.Fatalf("413 on a single-file batch must be per-file, not total: exit %d (stderr: %s)", code, errB.String())
	}
	if !hasLine(out.String(), "failed     1") {
		t.Errorf("summary must show the failed file:\n%s", out.String())
	}
	if !hasLine(errB.String(), "rejected") {
		t.Errorf("stderr must explain the rejection:\n%s", errB.String())
	}
}

func TestUploadOutageExhaustedRetriesIsTotalFailure(t *testing.T) {
	neutralizeEnv(t)
	m := newMockSulu(t)
	m.failFiles = 4 // exhausts all 4 attempts of the single batch
	dir := t.TempDir()
	writeFixture(t, dir, "a-result.json", "{}")
	var out, errB bytes.Buffer
	code := uploadNoBackoff(t, []string{"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1"}, &out, &errB)
	if code != 1 {
		t.Fatalf("exhausted retries on a normal-size single file must be a total failure, got %d", code)
	}
}

func TestUpload409StopsFurtherBatches(t *testing.T) {
	neutralizeEnv(t)
	m := newMockSulu(t)
	m.finished = true
	dir := t.TempDir()
	// 101 small files → two batches; the 409 on batch 1 must prevent batch 2
	for i := 0; i < 101; i++ {
		writeFixture(t, dir, fmt.Sprintf("f%03d-result.json", i), "{}")
	}
	var out, errB bytes.Buffer
	code := Upload([]string{"--results", dir, "--url", m.srv.URL, "--token", "t", "--project", "1"}, &out, &errB, "test")
	if code != 1 {
		t.Fatalf("want 1, got %d", code)
	}
	m.mu.Lock()
	uploads := m.uploads
	m.mu.Unlock()
	if uploads != 0 {
		t.Errorf("409 must stop the loop before any further batch lands, got %d", uploads)
	}
}

func TestUploadHelpExitsZero(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	if code := Upload([]string{"-h"}, &out, &errB, "test"); code != 0 {
		t.Fatalf("-h must exit 0, got %d", code)
	}
}

func TestUploadEmptyResultsFails(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	code := Upload([]string{"--results", t.TempDir(), "--url", "http://x", "--token", "t", "--project", "1"}, &out, &errB, "test")
	if code != 1 {
		t.Fatalf("want 1, got %d", code)
	}
}
