// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test && nvml

package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// WithMockNVML sets the singleton NVML library for testing purposes.
// This is useful to test the NVML library without having to initialize it
// manually. It automatically restores the original NVML library on test
func WithMockNVML(tb testing.TB, lib nvml.Interface) {
	singleton.mu.Lock()
	defer singleton.mu.Unlock()

	singleton.lib = lib
	singleton.isInitialized = true

	tb.Cleanup(func() {
		singleton.mu.Lock()
		defer singleton.mu.Unlock()

		singleton.lib = nil
		singleton.isInitialized = false
	})
}
