package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Handler-captured variables are written on httptest's server goroutines and read by
// the test goroutine; the in-process TCP hop gives the race detector no happens-before
// edge, so every capture below is guarded by a mutex (CI runs `go test -race`).

func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "tok-123", false, "test")
	c.Sleep = func(time.Duration) {} // no real backoff in tests
	return c
}

func TestCreateLaunch(t *testing.T) {
	var mu sync.Mutex
	var gotAuth, gotUA, gotPath string
	var gotBody LaunchRequest
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth, gotUA, gotPath = r.Header.Get("Authorization"), r.Header.Get("User-Agent"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"launchUuid":"u-1","launchId":42}`))
	}))
	s, err := c.CreateLaunch(LaunchRequest{ProjectID: 7, Name: "n", Tags: []string{"smoke"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.LaunchUUID != "u-1" || s.LaunchID != 42 {
		t.Errorf("bad session: %+v", s)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "Bearer tok-123" || gotUA != "suluctl/test" || gotPath != "/api/import/launches" {
		t.Errorf("auth=%q ua=%q path=%q", gotAuth, gotUA, gotPath)
	}
	if gotBody.ProjectID != 7 || len(gotBody.Tags) != 1 {
		t.Errorf("bad body: %+v", gotBody)
	}
}

func TestRetryOn500ThenSuccess(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"launchUuid":"u","launchId":1}`))
	}))
	if _, err := c.CreateLaunch(LaunchRequest{ProjectID: 1}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Errorf("want 3 calls, got %d", calls.Load())
	}
}

func TestRetryGivesUpAfter4Attempts(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	if _, err := c.CreateLaunch(LaunchRequest{ProjectID: 1}); err == nil {
		t.Fatal("want error")
	}
	if calls.Load() != 4 {
		t.Errorf("want 4 attempts, got %d", calls.Load())
	}
}

func TestFatal402NotRetriedAndMessageExtracted(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusPaymentRequired)
		// real 402 guard shape: human text under "error", no "message" key
		_, _ = w.Write([]byte(`{"error":"Workspace is in read-only mode","code":"WORKSPACE_READ_ONLY","upgradeUrl":"/app/billing","path":"/api/import/launches"}`))
	}))
	_, err := c.CreateLaunch(LaunchRequest{ProjectID: 1})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 402 || apiErr.Code != "WORKSPACE_READ_ONLY" || apiErr.Message != "Workspace is in read-only mode" {
		t.Errorf("bad APIError: %+v", apiErr)
	}
	if calls.Load() != 1 {
		t.Errorf("402 must not be retried: %d calls", calls.Load())
	}
}

func TestFinishSendsState(t *testing.T) {
	var mu sync.Mutex
	var body map[string]string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		body = map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Unlock()
	}))
	if err := c.Finish("u-1", "STOPPED"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if body["executionState"] != "STOPPED" {
		t.Errorf("got %v", body)
	}
	mu.Unlock()
	if err := c.Finish("u-1", ""); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(body) != 0 {
		t.Errorf("empty finish must send {}: %v", body)
	}
}

func TestLedgerDecodesBareArray(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("want GET, got %s", r.Method)
		}
		_, _ = w.Write([]byte(`[{"fileName":"a-result.json","kind":"ALLURE_RESULT","status":"PARSED","error":null}]`))
	}))
	rows, err := c.Ledger("u-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "PARSED" {
		t.Errorf("got %+v", rows)
	}
}

func TestUploadFilesMultipart(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a-result.json", "b-result.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"uuid":"`+name+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var fileNames []string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		for _, fh := range r.MultipartForm.File["files"] {
			fileNames = append(fileNames, fh.Filename)
		}
		mu.Unlock()
		_, _ = w.Write([]byte(`{"files":[{"fileName":"a-result.json","kind":"ALLURE_RESULT","status":"PARSED"},{"fileName":"b-result.json","kind":"ALLURE_RESULT","status":"PARSED"}]}`))
	}))
	res, err := c.UploadFiles("u-1", []string{
		filepath.Join(dir, "a-result.json"),
		filepath.Join(dir, "b-result.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Status != "PARSED" {
		t.Errorf("bad response decode: %+v", res)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(fileNames) != 2 {
		t.Fatalf("want 2 parts named files, got %v", fileNames)
	}
	if fileNames[0] != "a-result.json" || fileNames[1] != "b-result.json" {
		t.Errorf("file names must be base names: %v", fileNames)
	}
}

func TestUploadRetryRebuildsRequest(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a-result.json", "b-result.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int32
	var mu sync.Mutex
	var secondAttemptParts int
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		secondAttemptParts = len(r.MultipartForm.File["files"])
		mu.Unlock()
		_, _ = w.Write([]byte(`{"files":[]}`))
	}))
	if _, err := c.UploadFiles("u-1", []string{
		filepath.Join(dir, "a-result.json"),
		filepath.Join(dir, "b-result.json"),
	}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if secondAttemptParts != 2 {
		t.Errorf("retry must re-read both files from disk, got %d parts", secondAttemptParts)
	}
}

func TestUploadFilesSkipsUnopenable(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "ok-result.json")
	_ = os.WriteFile(good, []byte(`{}`), 0o644)
	var count atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(32 << 20)
		count.Store(int32(len(r.MultipartForm.File["files"])))
		_, _ = w.Write([]byte(`{"files":[]}`))
	}))
	// a vanished file must be skipped, not fail the batch (watch-mode race)
	_, err := c.UploadFiles("u-1", []string{filepath.Join(dir, "gone.json"), good})
	if err != nil {
		t.Fatal(err)
	}
	if count.Load() != 1 {
		t.Errorf("want 1 part, got %d", count.Load())
	}
}

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
