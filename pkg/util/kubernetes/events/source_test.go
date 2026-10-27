// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceType(t *testing.T) {
	tests := []struct {
		name                string
		reportingController string
		sourceComponent     string
		detectionEnabled    bool
		want                string
	}{
		{
			name:                "maps the reporting controller",
			reportingController: "datadog-operator-manager",
			detectionEnabled:    true,
			want:                "datadog operator",
		},
		{
			name:             "falls back on the source component, set by emitters of the core API",
			sourceComponent:  "datadog-operator-manager",
			detectionEnabled: true,
			want:             "datadog operator",
		},
		{
			name:                "the reporting controller takes precedence over the source component",
			reportingController: "karpenter",
			sourceComponent:     "datadog-operator-manager",
			detectionEnabled:    true,
			want:                "karpenter",
		},
		{
			name:                "unknown emitters are attributed to kubernetes",
			reportingController: "abcd-test-controller",
			sourceComponent:     "abcd-test-source",
			detectionEnabled:    true,
			want:                "kubernetes",
		},
		{
			name:                "detection disabled",
			reportingController: "datadog-operator-manager",
			sourceComponent:     "datadog-operator-manager",
			detectionEnabled:    false,
			want:                "kubernetes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SourceType(tt.reportingController, tt.sourceComponent, tt.detectionEnabled))
		})
	}
}
