// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package sync is utilities for synchronization
package sync

import "github.com/DataDog/datadog-agent/pkg/util/funcs"

// ResetGlobalPoolTelemetry resets the global pool telemetry to the default value, useful for testing
// to avoid telemetry values being set by other tests
func ResetGlobalPoolTelemetry() {
	globalPoolTelemetry = funcs.MemoizeArgNoError(newPoolTelemetry)
}
