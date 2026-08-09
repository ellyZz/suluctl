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

func TestPeekDoesNotConsume(t *testing.T) {
	c := New()
	w := c.Writer("INFO")
	w.Write([]byte("one\ntwo\nthree\n"))

	got := c.Peek(2)
	if len(got) != 2 || got[0].Message != "one" || got[1].Message != "two" {
		t.Fatalf("Peek(2) = %+v, want oldest two in order", got)
	}
	again := c.Peek(2)
	if len(again) != 2 || again[0].Message != "one" {
		t.Errorf("second Peek(2) = %+v, must not consume", again)
	}
	all := c.Peek(10)
	if len(all) != 3 || all[2].Message != "three" {
		t.Errorf("Peek(10) = %+v, want all three", all)
	}
}

func TestDiscardRemovesOldestAndFreesCapBudget(t *testing.T) {
	c := New()
	w := c.Writer("INFO")
	for i := 0; i < MaxLines; i++ {
		w.Write([]byte("x\n"))
	}
	if c.Truncated() {
		t.Fatal("exactly MaxLines lines must not truncate")
	}

	c.Discard(5)
	w.Write([]byte("a\nb\nc\nd\ne\n")) // freed budget: 5 new lines must fit
	if c.Truncated() {
		t.Error("adds after Discard freed the budget must not truncate")
	}
	got := c.Entries()
	if len(got) != MaxLines {
		t.Fatalf("want %d entries after discard+refill, got %d", MaxLines, len(got))
	}
	if got[0].Message != "x" || got[len(got)-1].Message != "e" {
		t.Errorf("order broken: first=%q last=%q", got[0].Message, got[len(got)-1].Message)
	}

	c.Discard(MaxLines + 100) // over-discard clamps, must not panic
	if n := len(c.Entries()); n != 0 {
		t.Errorf("over-discard must clamp to empty, got %d entries", n)
	}
}

func TestTruncatedStaysStickyAcrossDiscard(t *testing.T) {
	c := New()
	w := c.Writer("INFO")
	for i := 0; i < MaxLines+10; i++ {
		w.Write([]byte("x\n"))
	}
	if !c.Truncated() {
		t.Fatal("must truncate past the cap")
	}
	c.Discard(MaxLines)
	if !c.Truncated() {
		t.Error("Truncated must stay sticky after Discard — output WAS dropped")
	}
}

func TestForceEmitsOversizedNewlinelessWrite(t *testing.T) {
	c := New()
	w := c.Writer("INFO")
	big := make([]byte, maxLineBytes+100)
	for i := range big {
		big[i] = 'a'
	}
	w.Write(big) // no newline at all — must be force-emitted, not buffered forever
	if len(c.Entries()) == 0 {
		t.Fatal("oversized newline-less write must be force-emitted, not buffered indefinitely")
	}
}
