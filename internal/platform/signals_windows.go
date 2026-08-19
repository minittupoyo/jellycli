//go:build windows

package platform

import "os"

func TerminationSignals() []os.Signal { return []os.Signal{os.Interrupt} }
