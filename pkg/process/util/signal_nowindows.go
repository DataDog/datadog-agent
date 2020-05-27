// +build !windows

package util

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HandleSignals tells us whether we should exit.
func HandleSignals(exit chan struct{}) {
	sigIn := make(chan os.Signal, 100)
	signal.Notify(sigIn, syscall.SIGINT, syscall.SIGTERM)
	// unix only in all likelihood; but we don't care.
	for sig := range sigIn {
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			signal.Stop(sigIn)
			log.Criticalf("Caught signal '%s'; terminating.", sig)
			close(exit)
		}
	}
}
