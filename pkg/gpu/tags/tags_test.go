// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

package tags

import (
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
)

type mockNVML struct {
	nvml.Interface
	deviceCount int
	countError  nvml.Return
	initError   nvml.Return
}

func (m *mockNVML) DeviceGetCount() (int, nvml.Return) {
	return m.deviceCount, m.countError
}

func (m *mockNVML) Init() nvml.Return {
	return m.initError
}

func TestGetTags(t *testing.T) {
	tests := []struct {
		name     string
		nvml     mockNVML
		wantTags []string
	}{
		{
			name: "no GPUs",
			nvml: mockNVML{
				deviceCount: 0,
				countError:  nvml.SUCCESS,
			},
			wantTags: nil,
		},
		{
			name: "has GPUs",
			nvml: mockNVML{
				deviceCount: 2,
				countError:  nvml.SUCCESS,
			},
			wantTags: []string{"gpu_host:true"},
		},
		{
			name: "device count error",
			nvml: mockNVML{
				countError: nvml.ERROR_UNKNOWN,
			},
			wantTags: nil,
		},
		{
			name: "init error",
			nvml: mockNVML{
				initError:   nvml.ERROR_UNKNOWN,
				countError:  nvml.SUCCESS,
				deviceCount: 10, // If the init check is not correct, we'll report the tag and the test will fail
			},
			wantTags: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nvmlLibrary = nil
			gotTags := getTags(func(_ ...nvml.LibraryOption) nvml.Interface { return &tt.nvml })
			assert.Equal(t, tt.wantTags, gotTags)
		})
	}
}

// BenchmarkGetTags requires libnvidia-ml.so to be present on the host.
// This benchmark uses the real NVML library to measure actual performance,
// as loading the native library is potentially the main bottleneck.
func BenchmarkGetTags(b *testing.B) {
	if res := nvml.Init(); res != nvml.SUCCESS && res != nvml.ERROR_ALREADY_INITIALIZED {
		b.Fatalf("Failed to initialize NVML library")
	}
	_ = nvml.Shutdown()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		nvmlLibrary = nil
		_ = GetTags()
	}

	// Verify the function completes within 500ms
	b.StopTimer()
	nvmlLibrary = nil
	start := time.Now()
	GetTags()
	duration := time.Since(start)
	if duration > 500*time.Millisecond {
		b.Errorf("GetTags took %v, expected less than 500ms", duration)
	} else {
		b.Logf("GetTags took %v", duration)
	}
}

func TestEnsureNvmlLibrary(t *testing.T) {
	t.Run("library present", func(t *testing.T) {
		// reset nvmlLibrary to nil to avoid interference from previous tests
		nvmlLibrary = nil
		mockLib := &mockNVML{
			initError: nvml.SUCCESS,
		}
		newFunc := func(_ ...nvml.LibraryOption) nvml.Interface { return mockLib }
		assert.NoError(t, ensureNvmlLibrary(newFunc))
	})
	t.Run("library absent", func(t *testing.T) {
		// reset nvmlLibrary to nil to avoid interference from previous tests
		nvmlLibrary = nil
		mockLib := &mockNVML{
			initError: nvml.ERROR_LIBRARY_NOT_FOUND,
		}
		newFunc := func(_ ...nvml.LibraryOption) nvml.Interface { return mockLib }
		assert.Error(t, ensureNvmlLibrary(newFunc))
	})

	t.Run("library absent, second call fails too", func(t *testing.T) {
		// reset nvmlLibrary to nil to avoid interference from previous tests
		nvmlLibrary = nil
		mockLib := &mockNVML{
			initError: nvml.ERROR_LIBRARY_NOT_FOUND,
		}
		newFunc := func(_ ...nvml.LibraryOption) nvml.Interface { return mockLib }
		assert.Error(t, ensureNvmlLibrary(newFunc))
		assert.Error(t, ensureNvmlLibrary(newFunc))
	})

	t.Run("library absent, second call succeeds", func(t *testing.T) {
		// reset nvmlLibrary to nil to avoid interference from previous tests
		nvmlLibrary = nil
		mockLibFail := &mockNVML{
			initError: nvml.ERROR_LIBRARY_NOT_FOUND,
		}
		mockLibSuccess := &mockNVML{
			initError: nvml.SUCCESS,
		}
		failFunc := func(_ ...nvml.LibraryOption) nvml.Interface { return mockLibFail }
		successFunc := func(_ ...nvml.LibraryOption) nvml.Interface { return mockLibSuccess }
		assert.Error(t, ensureNvmlLibrary(failFunc))
		assert.NoError(t, ensureNvmlLibrary(successFunc))
	})
}
