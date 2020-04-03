// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package main

import (
	"context"
	"flag"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	_ "github.com/DataDog/datadog-agent/pkg/util/containers/providers/cgroup"
)

// main is the main application entry point
func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	flag.Parse()

	agent.Run(ctx)
}
