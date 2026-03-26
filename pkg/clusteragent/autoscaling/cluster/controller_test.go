// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/model"
)

func TestIsCreatedByDatadog(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name:     "no labels are present",
			labels:   map[string]string{},
			expected: false,
		},
		{
			name: "other labels are present",
			labels: map[string]string{
				"otherLabel": "otherValue",
			},
			expected: false,
		},
		{
			name: "created label is present",
			labels: map[string]string{
				model.DatadogCreatedLabelKey: "true",
			},
			expected: true,
		},
		{
			name: "created and other label is present",
			labels: map[string]string{
				model.DatadogCreatedLabelKey: "true",
				"otherLabel":                 "otherValue",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCreatedByDatadog(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}
