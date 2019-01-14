// +build !windows

package main

import (
	"context"
	_ "net/http/pprof"

	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

// main is the main application entry point
func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	runAgent(ctx)
}
