package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testClient(baseURL string) *Client {
	c := New(baseURL, "tok", false, "test")
	c.Sleep = func(time.Duration) {} // no backoff in tests
	return c
}

func TestResolveTestCase_found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects/7/test-cases/resolve" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "com.x.Pet.create" {
			t.Fatalf("key = %s", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("auth = %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"displayId":"PET-37","testId":"com.x.Pet.create","name":"create"}`))
	}))
	defer srv.Close()

	res, found, err := testClient(srv.URL).ResolveTestCase(7, "com.x.Pet.create")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if res.DisplayID != "PET-37" {
		t.Fatalf("displayId = %s", res.DisplayID)
	}
}

func TestResolveTestCase_notFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"x"}`))
	}))
	defer srv.Close()

	_, found, err := testClient(srv.URL).ResolveTestCase(7, "missing")
	if err != nil {
		t.Fatalf("404 must not be an error, got %v", err)
	}
	if found {
		t.Fatal("expected found=false")
	}
}
