// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjector

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type openDriverHandleType func() (windows.Handle, error)
type queryDriverCapabilitiesType func(handle windows.Handle) (uint32, error)
type queryDriverCountersType func(handle windows.Handle, request *DDInjectorCounterRequest, buffer unsafe.Pointer, bufferSize uint32) (uint32, error)

// OverrideOpenDriverHandle replaces opening the ddinjector driver with a mock implementation.
func OverrideOpenDriverHandle(fn openDriverHandleType) {
	doOpenDriverHandle = fn
}

// OverrideQueryDriverCapabilities replaces the device call to query capabilities with a mock implementation.
func OverrideQueryDriverCapabilities(fn queryDriverCapabilitiesType) {
	doQueryDriverCapabilities = fn
}

// OverrideQueryDriverCounters replaces the device call to query counters with a mock implementation.
func OverrideQueryDriverCounters(fn queryDriverCountersType) {
	doQueryDriverCounters = fn
}
