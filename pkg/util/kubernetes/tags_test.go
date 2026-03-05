// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetStandardTags(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{
			name: "nominal case",
			labels: map[string]string{
				"app.kubernetes.io/instance":   "cluster-agent",
				"app.kubernetes.io/managed-by": "datadog-operator",
				"app.kubernetes.io/name":       "datadog-agent-deployment",
				"app.kubernetes.io/part-of":    "datadog-agent",
				"app.kubernetes.io/version":    "0.3.1",
				"tags.datadoghq.com/env":       "prod",
				"tags.datadoghq.com/version":   "1.5.2",
				"tags.datadoghq.com/service":   "datadog",
			},
			want: []string{
				"env:prod",
				"version:1.5.2",
				"service:datadog",
			},
		},
		{
			name:   "nil input",
			labels: nil,
			want:   []string{},
		},
		{
			name: "standard tags not found",
			labels: map[string]string{
				"app.kubernetes.io/instance":   "cluster-agent",
				"app.kubernetes.io/managed-by": "datadog-operator",
				"app.kubernetes.io/name":       "datadog-agent-deployment",
				"app.kubernetes.io/part-of":    "datadog-agent",
				"app.kubernetes.io/version":    "0.3.1",
			},
			want: []string{},
		},
		{
			name: "standard tags not all found",
			labels: map[string]string{
				"app.kubernetes.io/instance":   "cluster-agent",
				"app.kubernetes.io/managed-by": "datadog-operator",
				"app.kubernetes.io/name":       "datadog-agent-deployment",
				"app.kubernetes.io/part-of":    "datadog-agent",
				"app.kubernetes.io/version":    "0.3.1",
				"tags.datadoghq.com/env":       "prod",
				"tags.datadoghq.com/service":   "datadog",
			},
			want: []string{
				"env:prod",
				"service:datadog",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetStandardTags(tt.labels)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
