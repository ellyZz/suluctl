package console

import (
	"testing"
	"time"
)

func TestLineBufferingBlankFilterAndPartialFlush(t *testing.T) {
	c := New()
	fixed := time.Date(2026, 6, 20, 22, 1, 44, 0, time.UTC)
	c.now = func() time.Time { return fixed }
	w := c.Writer("INFO")
	w.Write([]byte("hello "))        // partial line
	w.Write([]byte("world\n\n  \n")) // completes line 1, then two blank lines (dropped)
	w.Write([]byte("tail-no-nl"))    // partial, no newline yet
	w.Flush()                        // emits the trailing partial

	got := c.Entries()
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].Message != "hello world" || got[0].Level != "INFO" || !got[0].TS.Equal(fixed) {
		t.Errorf("entry0 = %+v", got[0])
	}
	if got[1].Message != "tail-no-nl" {
		t.Errorf("entry1 = %+v", got[1])
	}
}

func TestStripsTrailingCR(t *testing.T) {
	c := New()
	c.Writer("ERROR").Write([]byte("win line\r\n"))
	got := c.Entries()
	if len(got) != 1 || got[0].Message != "win line" || got[0].Level != "ERROR" {
		t.Errorf("got %+v", got)
	}
}

func TestCapTruncates(t *testing.T) {
	c := New()
	w := c.Writer("INFO")
	for i := 0; i < MaxLines+10; i++ {
		w.Write([]byte("x\n"))
	}
	if !c.Truncated() {
		t.Error("must mark truncated past the line cap")
	}
	if n := len(c.Entries()); n != MaxLines {
		t.Errorf("must cap at %d entries, got %d", MaxLines, n)
	}
}
