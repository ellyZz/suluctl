package syncids

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ellyZz/suluctl/internal/api"
	"github.com/ellyZz/suluctl/internal/initscaffold"
)

func TestRun_javaEndToEnd(t *testing.T) {
	dir := t.TempDir()
	src := "package petstore;\n\nimport org.testng.annotations.Test;\n\npublic class T {\n    @Test\n    public void a() {}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "T.java"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"displayId":"PET-3","testId":"petstore.T.a","name":"a"}`))
	}))
	defer srv.Close()
	client := api.New(srv.URL, "tok", false, "test")

	res, err := Run(Config{
		Client: client, ProjectID: 1, Kind: initscaffold.Kind("testng"),
		Dir: dir, Opts: Options{JavaPackage: "petstore"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 1 {
		t.Fatalf("written = %d (%+v)", res.Written, res)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "T.java"))
	if !strings.Contains(string(out), `@SuluTest(id = "PET-3")`) {
		t.Fatalf("not written:\n%s", out)
	}
}

func TestRun_notFoundIsSkippedNotError(t *testing.T) {
	dir := t.TempDir()
	src := "package p;\n\nimport org.testng.annotations.Test;\n\npublic class T {\n    @Test\n    public void a() {}\n}\n"
	_ = os.WriteFile(filepath.Join(dir, "T.java"), []byte(src), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	res, err := Run(Config{
		Client: api.New(srv.URL, "tok", false, "test"), ProjectID: 1,
		Kind: initscaffold.Kind("junit5"), Dir: dir, Opts: Options{JavaPackage: "p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.NotFound != 1 || res.Written != 0 || res.Errors != 0 {
		t.Fatalf("expected 1 not-found, 0 written, 0 errors; got %+v", res)
	}
}

func TestRun_dryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	src := "package p;\n\nimport org.testng.annotations.Test;\n\npublic class T {\n    @Test\n    public void a() {}\n}\n"
	path := filepath.Join(dir, "T.java")
	_ = os.WriteFile(path, []byte(src), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"displayId":"PET-3","testId":"p.T.a","name":"a"}`))
	}))
	defer srv.Close()

	res, _ := Run(Config{
		Client: api.New(srv.URL, "tok", false, "test"), ProjectID: 1,
		Kind: initscaffold.Kind("testng"), Dir: dir, Opts: Options{JavaPackage: "p"}, DryRun: true,
	})
	if res.Written != 1 {
		t.Fatalf("dry-run should still tally Written=1, got %+v", res)
	}
	after, _ := os.ReadFile(path)
	if string(after) != src {
		t.Fatal("dry-run must not modify the file")
	}
}
