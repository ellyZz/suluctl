package runner

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestExitCodePassthrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	code, interrupted, err := Run([]string{"sh", "-c", "exit 3"})
	if err != nil || interrupted || code != 3 {
		t.Errorf("code=%d interrupted=%v err=%v", code, interrupted, err)
	}
	code, _, err = Run([]string{"sh", "-c", "exit 0"})
	if err != nil || code != 0 {
		t.Errorf("code=%d err=%v", code, err)
	}
}

func TestStartFailure(t *testing.T) {
	code, _, err := Run([]string{"/nonexistent-binary-xyz"})
	if err == nil || code != 127 {
		t.Errorf("want 127 + err, got code=%d err=%v", code, err)
	}
}

func TestEmptyArgv(t *testing.T) {
	code, _, err := Run(nil)
	if err == nil || code != 127 {
		t.Errorf("want 127 + err, got %d %v", code, err)
	}
}

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
