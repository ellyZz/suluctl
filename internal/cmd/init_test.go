package cmd

import (
	"bytes"
	"testing"
)

func TestInitHelpExitsZero(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	if code := Init([]string{"-h"}, &out, &errB, "test"); code != 0 {
		t.Fatalf("help exit = %d, want 0; stderr=%s", code, errB.String())
	}
}

func TestInitUnknownFrameworkIsUsageError(t *testing.T) {
	neutralizeEnv(t)
	var out, errB bytes.Buffer
	code := Init([]string{"--framework", "nope", "--dry-run"}, &out, &errB, "test")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr=%s", code, errB.String())
	}
}
