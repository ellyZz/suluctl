package runner

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Run starts argv with stdio passthrough, forwards SIGINT/SIGTERM to the child,
// and blocks until it exits. Returns the child's exit code and whether a signal
// arrived. err is non-nil only when the child could not be started (code 127).
//
// Signal notes: an interactive Ctrl+C reaches the child via the terminal's
// process group as well — the extra forwarded SIGINT is harmless for test
// runners. A targeted `kill -TERM <suluctl>` (CI cancellation) reaches only us,
// so forwarding is required. Process.Signal is not implemented on Windows —
// the error is deliberately ignored there.
func Run(argv []string) (code int, interrupted bool, err error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

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
