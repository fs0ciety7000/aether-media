package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// asService reports whether the process was started by the Windows service
// manager (aether service install / aether agent install).
var asService, _ = svc.IsWindowsService()

func init() {
	if !asService {
		return
	}
	// A service has no console: send the output (logs) to AETHER_LOG_FILE.
	if p := os.Getenv("AETHER_LOG_FILE"); p != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err == nil {
			if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640); err == nil {
				os.Stdout, os.Stderr = f, f
				_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(f.Fd()))
				_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
			}
		}
	}
}

// signalContext is cancelled on Ctrl+C, and on a stop or shutdown request of
// the service manager when running as a Windows service. The returned stop
// function waits for the service manager to be told the service stopped.
func signalContext() (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if !asService {
		return ctx, stop
	}
	ctx, cancel := context.WithCancel(ctx)
	finished, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_ = svc.Run("aether", serviceHandler{cancel: cancel, finished: finished})
		cancel() // the service manager gave up on us
	}()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			cancel()
			close(finished)
			<-done
			stop()
		})
	}
}

type serviceHandler struct {
	cancel   context.CancelFunc
	finished <-chan struct{} // closed once the command has shut down
}

func (h serviceHandler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case <-h.finished:
			return false, 0
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				// The server needs up to ~15 s to drain streams and jobs.
				status <- svc.Status{State: svc.StopPending, WaitHint: 30000}
				h.cancel()
			}
		}
	}
}
