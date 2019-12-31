// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet,kubeapiserver

package hostinfo

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
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

func TestGetLabelsToTags(t *testing.T) {
	tests := []struct {
		name               string
		configLabelsAsTags map[string]string
		expectLabelsAsTags map[string]string
	}{
		{
			name: "no labels in config",
			expectLabelsAsTags: map[string]string{
				"kubernetes.io/role": "kube_node_role",
			},
		},
		{
			name: "override node role label",
			configLabelsAsTags: map[string]string{
				"kubernetes.io/role": "role",
			},
			expectLabelsAsTags: map[string]string{
				"kubernetes.io/role": "role",
			},
		},
		{
			name: "lower case all labels",
			configLabelsAsTags: map[string]string{
				"A": "a",
			},
			expectLabelsAsTags: map[string]string{
				"kubernetes.io/role": "kube_node_role",
				"a":                  "a",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := config.Mock()
			config.Set("kubernetes_node_labels_as_tags", test.configLabelsAsTags)

			actuaLabelsAsTags := getLabelsToTags()
			assert.Equal(t, test.expectLabelsAsTags, actuaLabelsAsTags)
		})
	}
}
