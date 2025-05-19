// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver || kubelet

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestFormatCPURequests(t *testing.T) {
	tests := []struct {
		name        string
		cpuRequests resource.Quantity
		want        float64
	}{
		{
			name:        "nil",
			cpuRequests: resource.Quantity{},
			want:        0.0,
		},
		{
			name:        "0",
			cpuRequests: resource.MustParse("0"),
			want:        0.0,
		},
		{
			name:        "250m",
			cpuRequests: resource.MustParse("250m"),
			want:        25,
		},
		{
			name:        "1 core",
			cpuRequests: resource.MustParse("1"),
			want:        100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpuReq := FormatCPURequests(tt.cpuRequests)
			assert.Equal(t, tt.want, *cpuReq)
		})
	}
}

func TestFormatMemoryRequests(t *testing.T) {
	tests := []struct {
		name      string
		memoryReq resource.Quantity
		want      uint64
	}{
		{
			name:      "nil",
			memoryReq: resource.Quantity{},
			want:      0,
		},
		{
			name:      "0",
			memoryReq: resource.MustParse("0"),
			want:      0,
		},
		{
			name:      "250",
			memoryReq: resource.MustParse("250Mi"),
			want:      250 * 1024 * 1024,
		},
		{
			name:      "1Gi",
			memoryReq: resource.MustParse("1Gi"),
			want:      1 * 1024 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memReq := FormatMemoryRequests(tt.memoryReq)
			assert.Equal(t, tt.want, *memReq)
		})
	}
}
