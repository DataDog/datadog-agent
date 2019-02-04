// +build !windows

package main

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
)

// main is the main application entry point
func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Handle stops properly
	go handleSignal(cancelFunc)

	agent.Run(ctx)
}
