// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjector

import (
	"golang.org/x/sys/windows"
)

type openDriverHandleType func() (windows.Handle, error)
type queryDriverCountersType func(handle windows.Handle, request *DDInjectorCounterRequest, counters *DDInjectorCountersV1) error

// OverrideOpenDriverHandle replaces opening the ddinjector driver with a mock implementation.
func OverrideOpenDriverHandle(fn openDriverHandleType) {
	doOpenDriverHandle = fn
}

// OverrideQueryDriverCounters replaces the device call to query counters with a mock implementation.
func OverrideQueryDriverCounters(fn queryDriverCountersType) {
	doQueryDriverCounters = fn
}
