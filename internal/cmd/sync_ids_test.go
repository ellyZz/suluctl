package cmd

import (
	"bytes"
	"testing"
)

func TestSyncIDs_missingConfig_returns2(t *testing.T) {
	t.Setenv("SULU_URL", "")
	t.Setenv("SULU_TOKEN", "")
	t.Setenv("SULU_PROJECT_ID", "")
	var out, errW bytes.Buffer
	if code := SyncIDs(nil, &out, &errW, "test"); code != 2 {
		t.Fatalf("missing-config exit = %d (want 2)", code)
	}
}

func TestSyncIDs_help_returns0(t *testing.T) {
	var out, errW bytes.Buffer
	if code := SyncIDs([]string{"-h"}, &out, &errW, "test"); code != 0 {
		t.Fatalf("help exit = %d", code)
	}
}

func TestSyncIDs_unknownFramework_returns2(t *testing.T) {
	t.Setenv("SULU_URL", "http://x")
	t.Setenv("SULU_TOKEN", "t")
	t.Setenv("SULU_PROJECT_ID", "1")
	var out, errW bytes.Buffer
	if code := SyncIDs([]string{"--framework", "nope"}, &out, &errW, "test"); code != 2 {
		t.Fatalf("unknown-framework exit = %d (want 2)", code)
	}
}
