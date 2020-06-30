// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package checks implements Compliance Agent checks
package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/stretchr/testify/assert"
)

func TestKubernetesNodeEligible(t *testing.T) {
	tests := []struct {
		selector       *compliance.HostSelector
		labels         map[string]string
		expectEligible bool
	}{
		{
			selector:       nil,
			expectEligible: true,
		},
		{
			selector: &compliance.HostSelector{
				KubernetesNodeRole: "master",
			},
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bar",
			},
			expectEligible: true,
		},
		{
			selector: &compliance.HostSelector{
				KubernetesNodeRole: "master",
				KubernetesNodeLabels: []compliance.KubeNodeSelector{
					{
						Label: "foo",
						Value: "bar",
					},
				},
			},
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bar",
			},
			expectEligible: true,
		},
		{
			selector: &compliance.HostSelector{
				KubernetesNodeRole: "master",
				KubernetesNodeLabels: []compliance.KubeNodeSelector{
					{
						Label: "foo",
						Value: "bar",
					},
				},
			},
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
				"foo":                            "bazbar",
			},
			expectEligible: false,
		},
	}

	for _, tt := range tests {
		builder := builder{}
		assert.Equal(t, tt.expectEligible, builder.isKubernetesNodeEligible(tt.selector, tt.labels))
	}
}
