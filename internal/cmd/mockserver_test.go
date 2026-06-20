package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// mockSulu implements the 4 import endpoints with an in-memory ledger.
type mockSulu struct {
	mu            sync.Mutex
	srv           *httptest.Server
	finished      bool
	failFiles     int // make the next N /files requests answer 500 (outage simulation)
	reject413Next int // make the next N /files requests answer 413
	finishReq     map[string]string
	createReq     map[string]any
	ledger        []map[string]string
	uploads       int
	logs          []map[string]any
}

func newMockSulu(t *testing.T) *mockSulu {
	t.Helper()
	m := &mockSulu{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/import/launches", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.createReq = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&m.createReq)
		m.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"launchUuid":"mock-uuid","launchId":42}`))
	})
	mux.HandleFunc("POST /api/import/launches/mock-uuid/files", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.failFiles > 0 {
			m.failFiles--
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if m.reject413Next > 0 {
			m.reject413Next--
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte(`{"timestamp":"x","status":413,"error":"Payload Too Large","message":"Maximum upload size exceeded","path":"/api/import/launches/mock-uuid/files"}`))
			return
		}
		if m.finished {
			w.WriteHeader(http.StatusConflict)
			// real wire shape: generic ErrorResponse — the marker is a prefix of "message", no "code" field
			_, _ = w.Write([]byte(`{"timestamp":"2026-06-12T10:00:00","status":409,"error":"Conflict","message":"IMPORT_LAUNCH_FINISHED: launch already finished; files are no longer accepted","path":"/api/import/launches/mock-uuid/files"}`))
			return
		}
		m.uploads++
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			t.Error(err)
		}
		var rows []map[string]string
		for _, fh := range r.MultipartForm.File["files"] {
			row := map[string]string{"fileName": fh.Filename, "kind": "ALLURE_RESULT", "status": "PARSED"}
			m.ledger = append(m.ledger, row)
			rows = append(rows, row)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": rows})
	})
	mux.HandleFunc("POST /api/import/launches/mock-uuid/finish", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.finished = true
		m.finishReq = map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&m.finishReq)
	})
	mux.HandleFunc("GET /api/import/launches/mock-uuid/files", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(m.ledger)
	})
	mux.HandleFunc("POST /api/launches/42/logs", func(w http.ResponseWriter, r *http.Request) {
		var batch []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&batch)
		m.mu.Lock()
		m.logs = append(m.logs, batch...)
		m.mu.Unlock()
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

// neutralizeEnv keeps the user's real SULU_* env out of hermetic tests.
func neutralizeEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SULU_URL", "SULU_TOKEN", "SULU_PROJECT_ID", "SULU_LAUNCH_NAME"} {
		t.Setenv(k, "")
	}
}

func hasLine(out, substr string) bool { return strings.Contains(out, substr) }
