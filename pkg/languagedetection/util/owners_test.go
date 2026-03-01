// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"reflect"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNamespacedBaseOwnerReference(t *testing.T) {

	tests := []struct {
		name     string
		input    pbgo.PodLanguageDetails
		expected NamespacedOwnerReference
	}{
		{
			name: "Case of replicaset",
			input: pbgo.PodLanguageDetails{
				Namespace:            "default",
				Name:                 "pod-a",
				ContainerDetails:     []*pbgo.ContainerLanguageDetails{},
				InitContainerDetails: []*pbgo.ContainerLanguageDetails{},
				Ownerref: &pbgo.KubeOwnerInfo{
					Kind: "ReplicaSet",
					Name: "dummyrs-1-2342347",
				},
			},
			expected: NewNamespacedOwnerReference("apps/v1", "Deployment", "dummyrs-1", "default"),
		},
		{
			name: "Case of statefulset in custom namespace",
			input: pbgo.PodLanguageDetails{
				Namespace:            "custom",
				Name:                 "pod-b",
				ContainerDetails:     []*pbgo.ContainerLanguageDetails{},
				InitContainerDetails: []*pbgo.ContainerLanguageDetails{},
				Ownerref: &pbgo.KubeOwnerInfo{
					Kind: "StatefulSet",
					Name: "dummy-statefulset-name",
				},
			},
			expected: NewNamespacedOwnerReference("apps/v1", "StatefulSet", "dummy-statefulset-name", "custom"),
		},
	}

	for i := range tests {
		t.Run(tests[i].name, func(t *testing.T) {
			actual := GetNamespacedBaseOwnerReference(&tests[i].input)
			assert.True(t, reflect.DeepEqual(tests[i].expected, actual))
		})
	}

}

func TestGetGVR(t *testing.T) {
	t.Run("valid deployment", func(t *testing.T) {
		ref := &NamespacedOwnerReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-deploy",
			Namespace:  "default",
		}
		gvr, err := GetGVR(ref)
		require.NoError(t, err)
		assert.Equal(t, "apps", gvr.Group)
		assert.Equal(t, "v1", gvr.Version)
		assert.Equal(t, "deployments", gvr.Resource)
	})

	t.Run("valid statefulset", func(t *testing.T) {
		ref := &NamespacedOwnerReference{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
			Name:       "my-sts",
			Namespace:  "default",
		}
		gvr, err := GetGVR(ref)
		require.NoError(t, err)
		assert.Equal(t, "apps", gvr.Group)
		assert.Equal(t, "v1", gvr.Version)
		assert.Equal(t, "statefulsets", gvr.Resource)
	})

	t.Run("invalid api version", func(t *testing.T) {
		ref := &NamespacedOwnerReference{
			APIVersion: "///invalid",
			Kind:       "Deployment",
			Name:       "my-deploy",
			Namespace:  "default",
		}
		_, err := GetGVR(ref)
		assert.Error(t, err)
	})
}

func TestNewNamespacedOwnerReference(t *testing.T) {
	ref := NewNamespacedOwnerReference("apps/v1", "Deployment", "my-deploy", "prod")
	assert.Equal(t, "apps/v1", ref.APIVersion)
	assert.Equal(t, "Deployment", ref.Kind)
	assert.Equal(t, "my-deploy", ref.Name)
	assert.Equal(t, "prod", ref.Namespace)
}
