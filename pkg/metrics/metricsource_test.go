// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricSourceAWSMicroVMString(t *testing.T) {
	tests := []struct {
		source   MetricSource
		expected string
	}{
		{MetricSourceAWSMicroVMCustom, "aws_microvm_custom"},
		{MetricSourceAWSMicroVMEnhanced, "aws_microvm_enhanced"},
		{MetricSourceAWSMicroVMRuntime, "aws_microvm_runtime"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.expected, tc.source.String())
	}
}
