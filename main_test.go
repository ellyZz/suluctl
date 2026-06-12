package main

import "testing"

func TestRunDispatch(t *testing.T) {
	if got := run([]string{"version"}); got != 0 {
		t.Errorf("version: want 0, got %d", got)
	}
	if got := run([]string{"bogus"}); got != 2 {
		t.Errorf("unknown command: want 2, got %d", got)
	}
	if got := run(nil); got != 2 {
		t.Errorf("no args: want 2, got %d", got)
	}
}
