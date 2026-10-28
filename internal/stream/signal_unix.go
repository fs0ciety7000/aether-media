//go:build unix

package stream

import (
	"os"
	"syscall"
)

// pauseProcess suspends FFmpeg (throttling) with SIGSTOP.
func pauseProcess(p *os.Process) error {
	if p == nil {
		return os.ErrProcessDone
	}
	return p.Signal(syscall.SIGSTOP)
}

// resumeProcess resumes a suspended FFmpeg with SIGCONT.
func resumeProcess(p *os.Process) error {
	if p == nil {
		return os.ErrProcessDone
	}
	return p.Signal(syscall.SIGCONT)
}
