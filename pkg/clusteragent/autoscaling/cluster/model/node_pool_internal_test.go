// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestNewNodePoolInternal_KarpenterV1(t *testing.T) {
	weight := int32(10)
	values := ClusterAutoscalingValues{
		TargetName: "my-target",
		TargetHash: "abc123",
		Type:       TypeKarpenterV1,
		Manifest: Manifest{
			KarpenterV1: &KarpenterV1NodePool{
				Metadata: Metadata{
					Name: "dd-my-pool",
					Labels: Labels{
						{Key: "env", Value: "prod"},
						{Key: "team", Value: "infra"},
					},
					Annotations: Annotations{
						{Key: "note", Value: "managed"},
					},
				},
				Spec: &karpenterv1.NodePoolSpec{
					Weight: &weight,
				},
			},
		},
	}

	npi := NewNodePoolInternal(values)

	assert.Equal(t, "dd-my-pool", npi.Name())
	assert.Equal(t, "my-target", npi.TargetName())
	assert.Equal(t, "abc123", npi.TargetHash())

	knp := npi.KarpenterNodePool()
	require.NotNil(t, knp)
	assert.Equal(t, "dd-my-pool", knp.Name)
	assert.Equal(t, "NodePool", knp.TypeMeta.Kind)
	assert.Equal(t, "karpenter.sh/v1", knp.TypeMeta.APIVersion)
	assert.Equal(t, map[string]string{"env": "prod", "team": "infra"}, knp.Labels)
	assert.Equal(t, map[string]string{"note": "managed"}, knp.Annotations)
	assert.Equal(t, &weight, knp.Spec.Weight)
	// NodeClaimTemplate metadata should mirror the NodePool metadata
	assert.Equal(t, map[string]string{"env": "prod", "team": "infra"}, knp.Spec.Template.ObjectMeta.Labels)
	assert.Equal(t, map[string]string{"note": "managed"}, knp.Spec.Template.ObjectMeta.Annotations)
}

func TestBuildKarpenterNodePoolFromManifest(t *testing.T) {
	weight := int32(5)
	kv1 := &KarpenterV1NodePool{
		Metadata: Metadata{
			Name: "test-pool",
			Labels: Labels{
				{Key: "foo", Value: "bar"},
			},
			Annotations: Annotations{
				{Key: "ann-key", Value: "ann-val"},
			},
		},
		Spec: &karpenterv1.NodePoolSpec{
			Weight: &weight,
		},
	}

	knp := buildKarpenterNodePoolFromManifest(kv1)

	assert.Equal(t, metav1.TypeMeta{Kind: "NodePool", APIVersion: "karpenter.sh/v1"}, knp.TypeMeta)
	assert.Equal(t, "test-pool", knp.Name)
	assert.Equal(t, map[string]string{"foo": "bar"}, knp.Labels)
	assert.Equal(t, map[string]string{"ann-key": "ann-val"}, knp.Annotations)
	assert.Equal(t, &weight, knp.Spec.Weight)
	// NodeClaimTemplate metadata mirrors the top-level metadata
	assert.Equal(t, map[string]string{"foo": "bar"}, knp.Spec.Template.ObjectMeta.Labels)
	assert.Equal(t, map[string]string{"ann-key": "ann-val"}, knp.Spec.Template.ObjectMeta.Annotations)
	// No Datadog-added labels
	assert.NotContains(t, knp.Labels, DatadogCreatedLabelKey)
}

func TestNewNodePoolInternal_MissingManifest(t *testing.T) {
	tests := []struct {
		name   string
		values ClusterAutoscalingValues
	}{
		{
			name: "type is karpenter v1 but manifest is nil",
			values: ClusterAutoscalingValues{
				TargetName: "target",
				TargetHash: "hash",
				Type:       TypeKarpenterV1,
				Manifest:   Manifest{KarpenterV1: nil},
			},
		},
		{
			name: "type is not karpenter v1",
			values: ClusterAutoscalingValues{
				TargetName: "target",
				TargetHash: "hash",
				Type:       "unknown/v1",
				Manifest: Manifest{
					KarpenterV1: &KarpenterV1NodePool{
						Metadata: Metadata{Name: "some-pool"},
						Spec:     &karpenterv1.NodePoolSpec{},
					},
				},
			},
		},
		{
			name:   "empty values",
			values: ClusterAutoscalingValues{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := NewNodePoolInternal(tt.values)
			assert.Nil(t, npi.KarpenterNodePool())
			assert.Empty(t, npi.Name())
		})
	}
}
