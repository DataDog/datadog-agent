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
		name           string
		expected       string
		labels         map[string]string
		kubeletVersion string
		kernelVersion  string
	}{
		{"AWS", "eks", map[string]string{"eks.amazonaws.com/compute-type": "large5n"}, "", ""},
		{"GCP", "gke", map[string]string{"cloud.google.com/gke-boot-disk": "/dev/ssd0n1"}, "", ""},
		{"Azure", "aks", map[string]string{"kubernetes.azure.com/mode": "managed-azure"}, "", ""},
		{"Unknown", "", map[string]string{"cloud.provider": "unknown"}, "", ""},
		// no labels detection by kubelet version
		{"AWS1", "eks", map[string]string{}, "v1.34.4-eks-efcacff", ""},
		{"GCP1", "gke", map[string]string{}, "v1.33.5-gke.2392000", ""},
		{"Azure1", "aks", map[string]string{}, "", "v1.27.7-azure-78901234"},
		{"Azure2", "aks", map[string]string{}, "", "azure-78901234"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := getKubeDistributionName(tc.labels, tc.kubeletVersion, tc.kernelVersion)
			assert.Equal(t, tc.expected, provider)
		})
	}
}
