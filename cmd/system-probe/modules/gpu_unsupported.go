// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (linux && (!linux_bpf || !nvml)) || windows

package modules

import "github.com/DataDog/datadog-agent/pkg/eventmonitor"

// createGPUProcessEventConsumer creates the process event consumer for the GPU module. Should be called from the event monitor module
func createGPUProcessEventConsumer(_ *eventmonitor.EventMonitor) error {
	return nil
}
