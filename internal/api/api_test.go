package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
