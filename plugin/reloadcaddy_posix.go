// +build !windows

package plugin

import (
	"os"
	"syscall"
)

func SignalReload() {
	self, _ := os.FindProcess(os.Getpid())
	self.Signal(syscall.SIGUSR1)
}
