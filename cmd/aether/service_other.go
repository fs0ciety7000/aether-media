//go:build !linux && !darwin && !windows

package main

import (
	"errors"
	"runtime"
)

var errServiceUnsupported = errors.New("installation en service non prise en charge sur " + runtime.GOOS + " : lancez « aether serve » ou « aether agent » avec le gestionnaire de services du système")

func (s *serviceSpec) resolve(bool) error        { return errServiceUnsupported }
func installSteps(serviceSpec) ([]step, error)   { return nil, errServiceUnsupported }
func uninstallSteps(serviceSpec) ([]step, error) { return nil, errServiceUnsupported }
func statusSteps(serviceSpec) ([]step, error)    { return nil, errServiceUnsupported }
func checkPrivileges(serviceSpec) error          { return errServiceUnsupported }
func installHints(serviceSpec) []string          { return nil }
