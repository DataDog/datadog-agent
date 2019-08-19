// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package main

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

// DefaultConfigPath specifies the default configuration file path for non-Windows systems.
const DefaultConfigPath = "/opt/datadog-agent/etc/datadog.yaml"

func runAgent() {
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	agent.Run(ctx)
}
