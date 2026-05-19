// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, map[string]string{"env": "prod"}, knp.Labels)
	assert.Equal(t, map[string]string{"note": "managed"}, knp.Annotations)
	assert.Equal(t, &weight, knp.Spec.Weight)
	assert.Empty(t, knp.Spec.Template.ObjectMeta.Labels)
	assert.Empty(t, knp.Spec.Template.ObjectMeta.Annotations)
}

func TestNewNodePoolInternal_MissingManifest(t *testing.T) {
	tests := []struct {
		name   string
		values ClusterAutoscalingValues
	}{
		{
			name: "wrong type",
			values: ClusterAutoscalingValues{
				TargetName: "target",
				TargetHash: "hash",
				// Type not set to TypeKarpenterV1
				Manifest: Manifest{
					KarpenterV1: &KarpenterV1NodePool{
						Metadata: Metadata{Name: "pool"},
						Spec:     &karpenterv1.NodePoolSpec{},
					},
				},
			},
		},
		{
			name: "nil KarpenterV1 manifest",
			values: ClusterAutoscalingValues{
				TargetName: "target",
				TargetHash: "hash",
				Type:       TypeKarpenterV1,
				Manifest:   Manifest{KarpenterV1: nil},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := NewNodePoolInternal(tt.values)
			assert.Nil(t, npi.KarpenterNodePool())
		})
	}
}

func TestBuildKarpenterNodePoolFromManifest(t *testing.T) {
	weight := int32(5)
	kv1 := &KarpenterV1NodePool{
		Metadata: Metadata{
			Name:        "test-pool",
			Labels:      Labels{{Key: "foo", Value: "bar"}},
			Annotations: Annotations{{Key: "ann-key", Value: "ann-val"}},
		},
		Spec: &karpenterv1.NodePoolSpec{Weight: &weight},
	}

	knp := buildKarpenterNodePoolFromManifest(kv1)

	require.NotNil(t, knp)
	assert.Equal(t, metav1.TypeMeta{Kind: "NodePool", APIVersion: "karpenter.sh/v1"}, knp.TypeMeta)
	assert.Equal(t, "test-pool", knp.Name)
	assert.Equal(t, map[string]string{"foo": "bar"}, knp.Labels)
	assert.Equal(t, map[string]string{"ann-key": "ann-val"}, knp.Annotations)
	assert.Equal(t, &weight, knp.Spec.Weight)
	assert.Empty(t, knp.Spec.Template.ObjectMeta.Labels)
	assert.Empty(t, knp.Spec.Template.ObjectMeta.Annotations)
	assert.NotContains(t, knp.Labels, DatadogCreatedLabelKey)
}

func TestBuildKarpenterNodePoolFromManifest_WithTemplateMetadata(t *testing.T) {
	kv1 := &KarpenterV1NodePool{
		Metadata: Metadata{
			Name:        "test-pool",
			Labels:      Labels{{Key: "foo", Value: "bar"}},
			Annotations: Annotations{{Key: "ann-key", Value: "ann-val"}},
		},
		TemplateMetadata: &Metadata{
			Labels:      Labels{{Key: "node-label", Value: "node-val"}},
			Annotations: Annotations{{Key: "node-ann", Value: "node-ann-val"}},
		},
		Spec: &karpenterv1.NodePoolSpec{},
	}

	knp := buildKarpenterNodePoolFromManifest(kv1)

	require.NotNil(t, knp)
	assert.Equal(t, map[string]string{"foo": "bar"}, knp.Labels)
	assert.Equal(t, map[string]string{"ann-key": "ann-val"}, knp.Annotations)
	assert.Equal(t, map[string]string{"node-label": "node-val"}, knp.Spec.Template.ObjectMeta.Labels)
	assert.Equal(t, map[string]string{"node-ann": "node-ann-val"}, knp.Spec.Template.ObjectMeta.Annotations)
}

func TestBuildKarpenterNodePoolFromManifest_TemplateMetadataBlocklist(t *testing.T) {
	kv1 := &KarpenterV1NodePool{
		Metadata: Metadata{Name: "test-pool"},
		TemplateMetadata: &Metadata{
			Labels: Labels{
				{Key: "app", Value: "my-app"},
				{Key: "karpenter.sh/capacity-type", Value: "spot"},
				{Key: "node-role.kubernetes.io/control-plane", Value: ""},
				{Key: "k8s.io/foo", Value: "bar"},
			},
			Annotations: Annotations{
				{Key: "team", Value: "infra"},
				{Key: "kubernetes.io/created-by", Value: "value"},
				{Key: "karpenter.sh/do-not-disrupt", Value: "true"},
			},
		},
		Spec: &karpenterv1.NodePoolSpec{},
	}

	knp := buildKarpenterNodePoolFromManifest(kv1)

	require.NotNil(t, knp)
	assert.Equal(t, map[string]string{"app": "my-app"}, knp.Spec.Template.ObjectMeta.Labels)
	assert.Equal(t, map[string]string{"team": "infra"}, knp.Spec.Template.ObjectMeta.Annotations)
}

func TestFilterTemplateMetadataKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    []KeyValue
		expected map[string]string
	}{
		{
			name:     "empty input",
			input:    nil,
			expected: map[string]string{},
		},
		{
			name: "all allowed",
			input: []KeyValue{
				{Key: "app", Value: "web"},
				{Key: "team", Value: "platform"},
			},
			expected: map[string]string{"app": "web", "team": "platform"},
		},
		{
			name: "keys that contain blocked strings but are not in blocked domains are allowed",
			input: []KeyValue{
				{Key: "teamk8s.io/role", Value: "worker"},
				{Key: "example.com/karpenter.sh-mode", Value: "fast"},
			},
			expected: map[string]string{"teamk8s.io/role": "worker", "example.com/karpenter.sh-mode": "fast"},
		},
		{
			name: "reserved labels are dropped",
			input: []KeyValue{
				{Key: "app", Value: "web"},
				{Key: "node-role.kubernetes.io/control-plane", Value: ""},
				{Key: "k8s.io/foo", Value: "bar"},
				{Key: "karpenter.sh/capacity-type", Value: "spot"},
				{Key: "karpenter.sh/injected", Value: "bad"},
			},
			expected: map[string]string{"app": "web"},
		},
		{
			name: "reserved annotations are dropped",
			input: []KeyValue{
				{Key: "team", Value: "infra"},
				{Key: "karpenter.sh/do-not-disrupt", Value: "true"},
				{Key: "kubernetes.io/created-by", Value: "value"},
			},
			expected: map[string]string{"team": "infra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, filterTemplateMetadataKeys(tt.input))
		})
	}
}

func TestBuildKarpenterNodePoolFromManifest_NilSpec(t *testing.T) {
	kv1 := &KarpenterV1NodePool{
		Metadata: Metadata{Name: "test-pool"},
		Spec:     nil,
	}
	assert.Nil(t, buildKarpenterNodePoolFromManifest(kv1))
}

func TestGetNodePoolWeight(t *testing.T) {
	tests := []struct {
		name           string
		weight         *int32
		expectedWeight int32
	}{
		{
			name:           "nil weight defaults to 1",
			weight:         nil,
			expectedWeight: 1,
		},
		{
			name:           "weight incremented by 1",
			weight:         func() *int32 { w := int32(5); return &w }(),
			expectedWeight: 6,
		},
		{
			name:           "weight capped at 100",
			weight:         func() *int32 { w := int32(99); return &w }(),
			expectedWeight: 100,
		},
		{
			name:           "weight at max stays at 100",
			weight:         func() *int32 { w := int32(100); return &w }(),
			expectedWeight: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			np := &karpenterv1.NodePool{}
			np.Spec.Weight = tt.weight
			assert.Equal(t, tt.expectedWeight, *GetNodePoolWeight(np))
		})
	}
}
