// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml && test

package nvml

import "github.com/NVIDIA/go-nvml/pkg/nvml"

// testingT is an interface matching the necessary testing.T methods we need
type testingT interface {
	Cleanup(func())
	Errorf(format string, args ...any)
}

// WithMockDeviceCache sets up a mock device cache for testing and ensures cleanup
// This should only be used in tests
func WithMockDeviceCache(t testingT, mock nvml.Interface) {
	if mock == nil {
		return
	}

	original, _ := GetDeviceCache()
	t.Cleanup(func() {
		resetDeviceCache(original)
	})
	mockCache, err := newDeviceCacheWithOptions(mock)
	if err != nil {
		t.Errorf("error creating mock device cache: %v", err)
	}
	resetDeviceCache(mockCache)

}

// For testing purposes only
func resetDeviceCache(dc DeviceCache) {
	initMutex.Lock()
	defer initMutex.Unlock()
	globalDeviceCache.Store(&dc)
}
