// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"flag"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// main is the main application entry point
func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())

	// prepare go runtime
	runtime.SetMaxProcs()
	if err := runtime.SetGoMemLimit(config.IsContainerized()); err != nil {
		log.Debugf("Couldn't set Go memory limit: %s", err)
	}

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	flag.Parse()

	Run(ctx)
}
