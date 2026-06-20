package runner

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Run is RunWithCapture with stdout/stderr inherited from the parent (no capture).
func Run(argv []string) (code int, interrupted bool, err error) {
	return RunWithCapture(argv, nil, nil)
}

// RunWithCapture starts argv, forwards SIGINT/SIGTERM to the child, and blocks until
// it exits. stdout/stderr, when non-nil, receive the child's streams — pass
// io.MultiWriter(os.Stdout, capture) to both echo and capture; nil inherits the
// parent's os.Stdout/os.Stderr. stdin is always inherited. Returns the child's exit
// code and whether a signal arrived; err is non-nil only when the child could not be
// started (code 127). A signal-killed child yields ExitCode() == -1.
//
// Signal notes: an interactive Ctrl+C reaches the child via the terminal's
// process group as well — the extra forwarded SIGINT is harmless for test
// runners. A targeted `kill -TERM <suluctl>` (CI cancellation) reaches only us,
// so forwarding is required. Process.Signal is not implemented on Windows —
// the error is deliberately ignored there.
func RunWithCapture(argv []string, stdout, stderr io.Writer) (code int, interrupted bool, err error) {
	if len(argv) == 0 {
		return 127, false, errors.New("empty command")
	}
	out := stdout
	if out == nil {
		out = os.Stdout
	}
	errW := stderr
	if errW == nil {
		errW = os.Stderr
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, errW

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		return 127, false, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case sig := <-sigCh:
			interrupted = true
			_ = cmd.Process.Signal(sig)
		case waitErr := <-done:
			if waitErr == nil {
				return 0, interrupted, nil
			}
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				return exitErr.ExitCode(), interrupted, nil
			}
			return 127, interrupted, waitErr
		}
	}
}
