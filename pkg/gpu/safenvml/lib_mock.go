// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test && nvml

package safenvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// allSymbols is a pre-initialized map containing all NVML API functions
// Critical symbols are required for the wrapper to work,
// while non-critical symbols are nice to have but not essential
var allSymbols = map[string]struct{}{}

func init() {
	// Add critical symbols first
	for _, symbol := range getCriticalAPIs() {
		allSymbols[symbol] = struct{}{}
	}

	// Add non-critical symbols
	for _, symbol := range getNonCriticalAPIs() {
		allSymbols[symbol] = struct{}{}
	}
}

// WithMockNVML calls the WithPartialMockNVML with all symbols available
func WithMockNVML(tb testing.TB, lib nvml.Interface) {
	WithPartialMockNVML(tb, lib, allSymbols)
}

func resetSingleton() {
	singleton.mu.Lock()
	defer singleton.mu.Unlock()

	singleton.lib = nil
	singleton.capabilities = nil
}

// WithPartialMockNVML sets the singleton SafeNVML library for testing purposes.
// This is useful to test the NVML library without having to initialize it
// manually. It automatically restores the original NVML library on test cleanup
func WithPartialMockNVML(tb testing.TB, lib nvml.Interface, capabilities map[string]struct{}) {
	singleton.mu.Lock()
	defer singleton.mu.Unlock()

	singleton.lib = lib
	singleton.capabilities = capabilities

	tb.Cleanup(resetSingleton)
}

// WithMockNvmlNewFunc overrides the function to create a new NVML library instance.
// It can be used to test the NVML library without having to initialize it
// manually. It automatically restores the original function on test cleanup
func WithMockNvmlNewFunc(tb testing.TB, f func(opts ...nvml.LibraryOption) nvml.Interface) {
	oldNvmlNewFunc := nvmlNewFunc
	nvmlNewFunc = f
	tb.Cleanup(func() {
		nvmlNewFunc = oldNvmlNewFunc
		resetSingleton()
	})
}
