// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package hostinfo

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type mockMetadataAsTags struct {
	nodeLabelsAsTags      map[string]string
	nodeAnnotationsAsTags map[string]string
}

var _ configutils.MetadataAsTags = &mockMetadataAsTags{}

// GetPodLabelsAsTags implements MetadataAsTags#GetPodLabelsAsTags
func (m *mockMetadataAsTags) GetPodLabelsAsTags() map[string]string {
	panic("not implemented")
}

// GetPodAnnotationsAsTags implements MetadataAsTags#GetPodAnnotationsAsTags
func (m *mockMetadataAsTags) GetPodAnnotationsAsTags() map[string]string {
	panic("not implemented")
}

// GetNodeLabelsAsTags implements MetadataAsTags#GetNodeLabelsAsTags
func (m *mockMetadataAsTags) GetNodeLabelsAsTags() map[string]string {
	return m.nodeLabelsAsTags
}

// GetNodeAnnotationsAsTags implements MetadataAsTags#GetNodeAnnotationsAsTags
func (m *mockMetadataAsTags) GetNodeAnnotationsAsTags() map[string]string {
	return m.nodeAnnotationsAsTags
}

// GetNamespaceLabelsAsTags implements MetadataAsTags#GetNamespaceLabelsAsTags
func (m *mockMetadataAsTags) GetNamespaceLabelsAsTags() map[string]string {
	panic("not implemented")
}

// GetNamespaceAnnotationsAsTags implements MetadataAsTags#GetNamespaceAnnotationsAsTags
func (m *mockMetadataAsTags) GetNamespaceAnnotationsAsTags() map[string]string {
	panic("not implemented")
}

// GetResourcesLabelsAsTags implements MetadataAsTags#GetResourcesLabelsAsTags
func (m *mockMetadataAsTags) GetResourcesLabelsAsTags() map[string]map[string]string {
	panic("not implemented")
}

// GetResourcesAnnotationsAsTags implements MetadataAsTags#GetResourcesAnnotationsAsTags
func (m *mockMetadataAsTags) GetResourcesAnnotationsAsTags() map[string]map[string]string {
	panic("not implemented")
}

func newMockMetadataAsTags(nodeLabelsAsTags, nodeAnnotationsAsTags map[string]string) configutils.MetadataAsTags {
	return &mockMetadataAsTags{
		nodeLabelsAsTags:      nodeLabelsAsTags,
		nodeAnnotationsAsTags: nodeAnnotationsAsTags,
	}
}

func TestKubeNodeTagsProvider__getNodeLabelsAsTags(t *testing.T) {
	labelsAsTagsFromConfig := map[string]string{
		"foo": "bar",
	}

	expectedNodeLabelsAsTags := map[string]string{
		"foo":               "bar",
		NormalizedRoleLabel: kubernetes.KubeNodeRoleTagName,
	}

	metadataAsTags := newMockMetadataAsTags(labelsAsTagsFromConfig, map[string]string{})
	kubeNodeTagsProvider := KubeNodeTagsProvider{metadataAsTags}
	labelsAsTags := kubeNodeTagsProvider.getNodeLabelsAsTags()
	assert.Truef(t, reflect.DeepEqual(labelsAsTags, expectedNodeLabelsAsTags), "Expected %v, found %v", expectedNodeLabelsAsTags, labelsAsTags)
}

func TestExtractTags(t *testing.T) {
	gkeLabels := map[string]string{
		"beta.kubernetes.io/arch":       "amd64",
		"beta.kubernetes.io/os":         "linux",
		"cloud.google.com/gke-nodepool": "default-pool",
		"kubernetes.io/hostname":        "gke-dummy-18-default-pool-6888842e-hcv0",
	}

	roleLabels := map[string]string{
		"kubernetes.io/role":                   "compute-node2",
		"node-role.kubernetes.io":              "foo",
		"node-role.kubernetes.io/9090-090-9":   "bar",
		"node-role.kubernetes.io/compute-node": "",
		"node-role.kubernetes.io/foo":          "bar",
	}

	gkeLabelsWithRole := map[string]string{
		"beta.kubernetes.io/arch":       "amd64",
		"beta.kubernetes.io/os":         "linux",
		"cloud.google.com/gke-nodepool": "default-pool",
		"kubernetes.io/hostname":        "gke-dummy-18-default-pool-6888842e-hcv0",
		"kubernetes.io/role":            "foo",
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
		{
			nodeLabels:   roleLabels,
			labelsToTags: getDefaultLabelsToTags(),
			expectedTags: []string{
				"kube_node_role:compute-node2",
				"kube_node_role:9090-090-9",
				"kube_node_role:compute-node",
				"kube_node_role:foo",
			},
		},
		{
			nodeLabels: gkeLabelsWithRole,
			labelsToTags: map[string]string{
				"*": "foo_%%label%%",
			},
			expectedTags: []string{
				"foo_beta.kubernetes.io/arch:amd64",
				"foo_beta.kubernetes.io/os:linux",
				"foo_cloud.google.com/gke-nodepool:default-pool",
				"foo_kubernetes.io/hostname:gke-dummy-18-default-pool-6888842e-hcv0",
				"foo_kubernetes.io/role:foo",
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			tags := extractTags(tc.nodeLabels, tc.labelsToTags)
			assert.ElementsMatch(t, tc.expectedTags, tags)
		})
	}
}
