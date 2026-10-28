//go:build !windows

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// signalContext is cancelled on Ctrl+C or SIGTERM (systemd, launchd, Docker).
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
