// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSimpleGPUName(t *testing.T) {
	tests := []struct {
		name     string
		gpuName  ResourceGPU
		found    bool
		expected string
	}{
		{
			name:     "known gpu resource",
			gpuName:  gpuNvidiaGeneric,
			found:    true,
			expected: "nvidia",
		},
		{
			name:     "unknown gpu resource",
			gpuName:  ResourceGPU("cpu"),
			found:    false,
			expected: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, found := ExtractSimpleGPUName(test.gpuName)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.expected, actual)
		})
	}
}
