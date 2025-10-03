// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKubeDistributionName(t *testing.T) {
	// Test cases for cloud provider name
	testCases := []struct {
		name     string
		expected string
		labels   map[string]string
	}{
		{"AWS", "eks", map[string]string{"eks.amazonaws.com/compute-type": "large5n"}},
		{"GCP", "gke", map[string]string{"cloud.google.com/gke-boot-disk": "/dev/ssd0n1"}},
		{"Azure", "aks", map[string]string{"kubernetes.azure.com/mode": "managed-azure"}},
		{"Unknown", "", map[string]string{"cloud.provider": "unknown"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := getKubeDistributionNameFromNodeLabels(tc.labels)
			assert.Equal(t, tc.expected, provider)
		})
	}
}
