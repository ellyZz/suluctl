// Package console captures a wrapped command's stdout/stderr line-by-line so
// `suluctl watch` can ship it to Sulu as launch-scoped logs. Bounded in memory.
package console

import (
	"bytes"
	"strings"
	"sync"
	"time"
)

// Caps: past either, capture stops and Truncated() reports true. The flush-once
// -at-exit model makes an on-disk spill unnecessary at this size.
const (
	MaxLines = 200_000
	MaxBytes = 50 << 20 // 50 MB
)

// Entry is one captured, non-blank line.
type Entry struct {
	TS      time.Time
	Level   string // "INFO" (stdout) | "ERROR" (stderr)
	Message string
}

// Capturer accumulates lines from one or more LineWriters, bounded by the caps.
type Capturer struct {
	mu        sync.Mutex
	entries   []Entry
	bytes     int
	truncated bool
	now       func() time.Time
}

func New() *Capturer { return &Capturer{now: time.Now} }

// Writer returns an io.Writer recording complete, non-blank lines at the given level.
func (c *Capturer) Writer(level string) *LineWriter { return &LineWriter{cap: c, level: level} }

func (c *Capturer) add(level, msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.truncated || len(c.entries) >= MaxLines || c.bytes+len(msg) > MaxBytes {
		c.truncated = true
		return
	}
	c.entries = append(c.entries, Entry{TS: c.now(), Level: level, Message: msg})
	c.bytes += len(msg)
}

// Entries returns a snapshot. Call after flushing every LineWriter.
func (c *Capturer) Entries() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Entry, len(c.entries))
	copy(out, c.entries)
	return out
}

// Truncated reports whether the cap was hit (some output dropped).
func (c *Capturer) Truncated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.truncated
}

// LineWriter is an io.Writer emitting one Entry per complete line.
type LineWriter struct {
	cap   *Capturer
	level string
	mu    sync.Mutex
	buf   []byte
}

func (w *LineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// Flush emits any trailing partial line (no newline at EOF). Call once after the child exits.
func (w *LineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(w.buf)
		w.buf = nil
	}
}

func (w *LineWriter) emit(line []byte) {
	s := strings.TrimRight(string(line), "\r")
	if strings.TrimSpace(s) == "" {
		return // backend message is @NotBlank — never record empty lines
	}
	w.cap.add(w.level, s)
}
