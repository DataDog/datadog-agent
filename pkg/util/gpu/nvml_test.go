// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

func TestGpuArchToString(t *testing.T) {
	tests := []struct {
		arch     nvml.DeviceArchitecture
		expected string
	}{
		{nvml.DEVICE_ARCH_KEPLER, "kepler"},
		{nvml.DEVICE_ARCH_UNKNOWN, "unknown"},
		{nvml.DeviceArchitecture(3751), "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, ArchToString(tt.arch))
		})
	}
}
