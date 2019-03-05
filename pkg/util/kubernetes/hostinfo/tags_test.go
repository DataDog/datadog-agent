// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet,kubeapiserver

package hostinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractTags(t *testing.T) {
	gkeLabels := map[string]string{
		"beta.kubernetes.io/arch":       "amd64",
		"beta.kubernetes.io/os":         "linux",
		"cloud.google.com/gke-nodepool": "default-pool",
		"kubernetes.io/hostname":        "gke-dummy-18-default-pool-6888842e-hcv0",
	}

	for _, tc := range []struct {
		nodeLabels   map[string]string
		labelsToTags map[string]string
		expectedTags []string
	}{
		{
			nodeLabels:   map[string]string{},
			labelsToTags: map[string]string{},
			expectedTags: nil,
		},
		{
			nodeLabels: gkeLabels,
			labelsToTags: map[string]string{
				"kubernetes.io/hostname": "nodename",
				"beta.kubernetes.io/os":  "os",
			},
			expectedTags: []string{
				"nodename:gke-dummy-18-default-pool-6888842e-hcv0",
				"os:linux",
			},
		},
		{
			nodeLabels: gkeLabels,
			labelsToTags: map[string]string{
				"kubernetes.io/hostname": "nodename",
				"beta.kubernetes.io/os":  "os",
			},
			expectedTags: []string{
				"nodename:gke-dummy-18-default-pool-6888842e-hcv0",
				"os:linux",
			},
		},
		{
			nodeLabels: map[string]string{},
			labelsToTags: map[string]string{
				"kubernetes.io/hostname": "nodename",
				"beta.kubernetes.io/os":  "os",
			},
			expectedTags: nil,
		},
		{
			nodeLabels:   gkeLabels,
			labelsToTags: map[string]string{},
			expectedTags: nil,
		},
	} {
		t.Run("", func(t *testing.T) {
			tags := extractTags(tc.nodeLabels, tc.labelsToTags)
			assert.ElementsMatch(t, tc.expectedTags, tags)
		})
	}
}
