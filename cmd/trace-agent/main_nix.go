// +build !windows

package main

import (
	"context"

	"github.com/StackVista/stackstate-agent/pkg/trace/agent"
	"github.com/StackVista/stackstate-agent/pkg/trace/watchdog"
)

// main is the main application entry point
func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	agent.Run(ctx)
}
