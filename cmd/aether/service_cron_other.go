//go:build !(linux || darwin || freebsd || openbsd || netbsd)

package main

import (
	"errors"
	"runtime"
	"time"
)

var errCronUnsupported = errors.New("installation par crontab non prise en charge sur " + runtime.GOOS)

func cronRun([]string) error                       { return errCronUnsupported }
func cronAgentRunning(cronPaths) (int, bool)       { return 0, false }
func stopCronAgent(cronPaths, time.Duration) error { return errCronUnsupported }
