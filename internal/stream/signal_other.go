//go:build !unix

package stream

import (
	"errors"
	"os"
)

var errNoSuspend = errors.New("stream: process suspension not supported on this platform")

// pauseProcess is not supported outside Unix: FFmpeg simply keeps running.
func pauseProcess(*os.Process) error { return errNoSuspend }

// resumeProcess is a no-op outside Unix.
func resumeProcess(*os.Process) error { return nil }
