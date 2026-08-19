//go:build !windows

package platform

import (
	"os"
	"syscall"
)

func TerminationSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
