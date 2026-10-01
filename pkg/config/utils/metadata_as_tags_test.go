// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package utils

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetMetadataAsTagsNoError(t *testing.T) {

	tests := []struct {
		name                       string
		podLabelsAsTags            map[string]string
		podAnnotationsAsTags       map[string]string
		nodeLabelsAsTags           map[string]string
		nodeAnnotationsAsTags      map[string]string
		namespaceLabelsAsTags      map[string]string
		namespaceAnnotationsAsTags map[string]string
		resourcesLabelsAsTags      string
		resourcesAnnotationsAsTags string
		expectedLabelsAsTags       map[string]map[string]string
		expectedAnnotationsAsTags  map[string]map[string]string
	}{
		{
			name:                       "no configs",
			resourcesLabelsAsTags:      "{}",
			resourcesAnnotationsAsTags: "{}",
			expectedLabelsAsTags:       map[string]map[string]string{},
			expectedAnnotationsAsTags:  map[string]map[string]string{},
		},
		{
			name:                       "empty string configs (serverless environments)",
			resourcesLabelsAsTags:      "",
			resourcesAnnotationsAsTags: "",
			expectedLabelsAsTags:       map[string]map[string]string{},
			expectedAnnotationsAsTags:  map[string]map[string]string{},
		},
		{
			name:                       "old configurations only",
			podLabelsAsTags:            map[string]string{"l1": "v1", "l2": "v2"},
			podAnnotationsAsTags:       map[string]string{"l3": "v3", "l4": "v4"},
			nodeLabelsAsTags:           map[string]string{"L5": "v5", "L6": "v6"}, // keys should be lower-cased automatically
			nodeAnnotationsAsTags:      map[string]string{"l7": "v7", "l8": "v8"},
			namespaceLabelsAsTags:      map[string]string{"l9": "v9", "l10": "v10"},
			namespaceAnnotationsAsTags: map[string]string{"l11": "v11", "l12": "v12"},
			resourcesLabelsAsTags:      "{}",
			resourcesAnnotationsAsTags: "{}",
			expectedLabelsAsTags: map[string]map[string]string{
				"nodes":      {"l5": "v5", "l6": "v6"},
				"pods":       {"l1": "v1", "l2": "v2"},
				"namespaces": {"l9": "v9", "l10": "v10"},
			},
			expectedAnnotationsAsTags: map[string]map[string]string{
				"nodes":      {"l7": "v7", "l8": "v8"},
				"pods":       {"l3": "v3", "l4": "v4"},
				"namespaces": {"l11": "v11", "l12": "v12"},
			},
		},
		{
			name:                       "new configurations only",
			resourcesLabelsAsTags:      `{"pods.": {"l1": "v1", "l2": "v2"}, "deployments.apps": {"l3": "v3", "l4": "v4"}, "namespaces": {"l5": "v5"}}`,
			resourcesAnnotationsAsTags: `{"nodes.": {"l6": "v6", "l7": "v7"},"deployments.apps": {"l8": "v8", "l9": "v9"}, "namespaces": {"l10": "v10"}}`,
			expectedLabelsAsTags: map[string]map[string]string{
				"pods":             {"l1": "v1", "l2": "v2"},
				"deployments.apps": {"l3": "v3", "l4": "v4"},
				"namespaces":       {"l5": "v5"},
			},
			expectedAnnotationsAsTags: map[string]map[string]string{
				"nodes":            {"l6": "v6", "l7": "v7"},
				"deployments.apps": {"l8": "v8", "l9": "v9"},
				"namespaces":       {"l10": "v10"},
			},
		},
		{
			name:                       "old and new configurations | new configuration should take precedence",
			podLabelsAsTags:            map[string]string{"l1": "v1", "l2": "v2"},
			podAnnotationsAsTags:       map[string]string{"l3": "v3", "l4": "v4"},
			nodeLabelsAsTags:           map[string]string{"l5": "v5", "l6": "v6"},
			nodeAnnotationsAsTags:      map[string]string{"l7": "v7", "l8": "v8"},
			namespaceLabelsAsTags:      map[string]string{"l9": "v9", "l10": "v10"},
			namespaceAnnotationsAsTags: map[string]string{"l11": "v11", "l12": "v12"},
			resourcesLabelsAsTags:      `{"pods.": {"l1": "x1", "l99": "v99"}, "deployments.apps": {"l3": "v3", "l4": "v4"}}`,
			resourcesAnnotationsAsTags: `{"nodes.": {"l6": "v6", "l7": "x7"}}`,
			expectedLabelsAsTags: map[string]map[string]string{
				"nodes":            {"l5": "v5", "l6": "v6"},
				"pods":             {"l1": "x1", "l2": "v2", "l99": "v99"},
				"deployments.apps": {"l3": "v3", "l4": "v4"},
				"namespaces":       {"l9": "v9", "l10": "v10"},
			},
			expectedAnnotationsAsTags: map[string]map[string]string{
				"nodes":      {"l6": "v6", "l7": "x7", "l8": "v8"},
				"pods":       {"l3": "v3", "l4": "v4"},
				"namespaces": {"l11": "v11", "l12": "v12"},
			},
		},
	}

	for _, test := range tests {

		t.Run(test.name, func(tt *testing.T) {
			mockConfig := configmock.New(t)

			mockConfig.SetInTest("kubernetes_pod_labels_as_tags", test.podLabelsAsTags)
			mockConfig.SetInTest("kubernetes_pod_annotations_as_tags", test.podAnnotationsAsTags)
			mockConfig.SetInTest("kubernetes_namespace_labels_as_tags", test.namespaceLabelsAsTags)
			mockConfig.SetInTest("kubernetes_namespace_annotations_as_tags", test.namespaceAnnotationsAsTags)
			mockConfig.SetInTest("kubernetes_node_labels_as_tags", test.nodeLabelsAsTags)
			mockConfig.SetInTest("kubernetes_node_annotations_as_tags", test.nodeAnnotationsAsTags)
			mockConfig.SetInTest("kubernetes_resources_labels_as_tags", test.resourcesLabelsAsTags)
			mockConfig.SetInTest("kubernetes_resources_annotations_as_tags", test.resourcesAnnotationsAsTags)

			metadataAsTags := GetMetadataAsTags(mockConfig)

			assert.NotNil(tt, metadataAsTags)

			labelsAsTags := metadataAsTags.GetResourcesLabelsAsTags()
			assert.Truef(tt, reflect.DeepEqual(labelsAsTags, test.expectedLabelsAsTags), "Expected %v, found %v", test.expectedLabelsAsTags, labelsAsTags)

			annotationsAsTags := metadataAsTags.GetResourcesAnnotationsAsTags()
			assert.Truef(tt, reflect.DeepEqual(annotationsAsTags, test.expectedAnnotationsAsTags), "Expected %v, found %v", test.expectedAnnotationsAsTags, annotationsAsTags)
		})
	}
}

func TestGetMetadataAsTagsGPUJobs(t *testing.T) {
	tests := []struct {
		name                         string
		gpuEnabled                   bool
		runKey                       string
		runType                      string
		groupKey                     string
		groupType                    string
		podLabelsAsTags              map[string]string
		resourcesAnnotationsAsTag    string
		expectedPodLabelsAsTags      map[string]string
		expectedPodAnnotationsAsTags map[string]string
	}{
		{
			name:       "gpu disabled",
			gpuEnabled: false,
			runKey:     "example/run",
			runType:    "label",
			groupKey:   "example/group",
			groupType:  "annotation",
		},
		{
			name:       "no keys configured",
			gpuEnabled: true,
			runType:    "label",
			groupType:  "label",
		},
		{
			name:                         "label and annotation",
			gpuEnabled:                   true,
			runKey:                       "Example/Run",
			runType:                      "label",
			groupKey:                     "example/group",
			groupType:                    "Annotation",
			expectedPodLabelsAsTags:      map[string]string{"example/run": "training_run_id"},
			expectedPodAnnotationsAsTags: map[string]string{"example/group": "training_group_id"},
		},
		{
			name:                         "env type is skipped",
			gpuEnabled:                   true,
			runKey:                       "_RAY_SUBMISSION_ID",
			runType:                      "env",
			groupKey:                     "example/group",
			groupType:                    "annotation",
			expectedPodAnnotationsAsTags: map[string]string{"example/group": "training_group_id"},
		},
		{
			name:                         "merged with existing user config",
			gpuEnabled:                   true,
			runKey:                       "example/run",
			runType:                      "label",
			groupKey:                     "example/group",
			groupType:                    "annotation",
			podLabelsAsTags:              map[string]string{"example/run": "my_run", "app": "app"},
			resourcesAnnotationsAsTag:    `{"pods":{"other":"other"}}`,
			expectedPodLabelsAsTags:      map[string]string{"example/run": "my_run,training_run_id", "app": "app"},
			expectedPodAnnotationsAsTags: map[string]string{"other": "other", "example/group": "training_group_id"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mockConfig := configmock.New(tt)
			mockConfig.SetInTest("gpu.enabled", test.gpuEnabled)
			mockConfig.SetInTest("gpu.jobs.run.key", test.runKey)
			mockConfig.SetInTest("gpu.jobs.run.type", test.runType)
			mockConfig.SetInTest("gpu.jobs.group.key", test.groupKey)
			mockConfig.SetInTest("gpu.jobs.group.type", test.groupType)
			mockConfig.SetInTest("kubernetes_pod_labels_as_tags", test.podLabelsAsTags)
			mockConfig.SetInTest("kubernetes_resources_annotations_as_tags", test.resourcesAnnotationsAsTag)

			metadataAsTags := GetMetadataAsTags(mockConfig)

			assertTagsEqual(tt, test.expectedPodLabelsAsTags, metadataAsTags.GetPodLabelsAsTags())
			assertTagsEqual(tt, test.expectedPodAnnotationsAsTags, metadataAsTags.GetPodAnnotationsAsTags())

			// Calling again must not accumulate tags
			assertTagsEqual(tt, test.expectedPodLabelsAsTags, GetMetadataAsTags(mockConfig).GetPodLabelsAsTags())
		})
	}
}

// assertTagsEqual treats nil and empty maps as equal
func assertTagsEqual(t *testing.T, expected, actual map[string]string) {
	if len(expected) == 0 {
		assert.Empty(t, actual)
		return
	}
	assert.Equal(t, expected, actual)
}
