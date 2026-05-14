// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
			assert.Equal(t, tt.expectedWeight, *getNodePoolWeight(np))
		})
	}
}

func targetNodePoolFixture() *karpenterv1.NodePool {
	weight := int32(5)
	terminationGrace := metav1.Duration{Duration: 30 * time.Minute}
	expireAfterDur := 24 * time.Hour
	consolidateAfterDur := 30 * time.Second
	return &karpenterv1.NodePool{
		TypeMeta: metav1.TypeMeta{Kind: "NodePool", APIVersion: "karpenter.sh/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "user-pool",
			Labels:            map[string]string{"target-label": "target"},
			Annotations:       map[string]string{"target-ann": "target"},
			ResourceVersion:   "12345",
			UID:               "11111111-2222-3333-4444-555555555555",
			Generation:        7,
			CreationTimestamp: metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			ManagedFields:     []metav1.ManagedFieldsEntry{{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply}},
		},
		Spec: karpenterv1.NodePoolSpec{
			Weight: &weight,
			Limits: karpenterv1.Limits{corev1.ResourceCPU: resource.MustParse("100")},
			Disruption: karpenterv1.Disruption{
				ConsolidationPolicy: karpenterv1.ConsolidationPolicyWhenEmpty,
				ConsolidateAfter:    karpenterv1.NillableDuration{Duration: &consolidateAfterDur},
			},
			Template: karpenterv1.NodeClaimTemplate{
				ObjectMeta: karpenterv1.ObjectMeta{
					Labels:      map[string]string{"tmpl-label": "target", "shared": "target"},
					Annotations: map[string]string{"tmpl-ann": "target"},
				},
				Spec: karpenterv1.NodeClaimTemplateSpec{
					NodeClassRef: &karpenterv1.NodeClassReference{Group: "karpenter.k8s.aws", Kind: "EC2NodeClass", Name: "target-nc"},
					Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
						{Key: "node.kubernetes.io/instance-type", Operator: corev1.NodeSelectorOpIn, Values: []string{"m5.large"}},
					},
					Taints:                 []corev1.Taint{{Key: "target-taint", Effect: corev1.TaintEffectNoSchedule}},
					TerminationGracePeriod: &terminationGrace,
					ExpireAfter:            karpenterv1.NillableDuration{Duration: &expireAfterDur},
				},
			},
		},
		Status: karpenterv1.NodePoolStatus{
			Resources: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("12")},
		},
	}
}

func rcNodePoolFromManifest(t *testing.T, m *KarpenterV1NodePool) NodePoolInternal {
	t.Helper()
	npi := NewNodePoolInternal(ClusterAutoscalingValues{
		TargetName: "user-pool",
		TargetHash: "h1",
		Type:       TypeKarpenterV1,
		Manifest:   Manifest{KarpenterV1: m},
	})
	require.NotNil(t, npi.KarpenterNodePool())
	return npi
}

func TestBuildReplicaNodePool_NilInputs(t *testing.T) {
	t.Run("nil target", func(t *testing.T) {
		npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
			Metadata: Metadata{Name: "dd-pool"},
			Spec:     &karpenterv1.NodePoolSpec{},
		})
		assert.Nil(t, npi.BuildReplicaNodePool(nil))
	})
	t.Run("no manifest", func(t *testing.T) {
		npi := NewNodePoolInternal(ClusterAutoscalingValues{TargetName: "user-pool"})
		assert.Nil(t, npi.BuildReplicaNodePool(targetNodePoolFixture()))
	})
}

func TestBuildReplicaNodePool_AlwaysReplacedFields(t *testing.T) {
	target := targetNodePoolFixture()
	npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
		Metadata: Metadata{
			Name:        "dd-pool",
			Labels:      Labels{{Key: "rc-label", Value: "rc"}},
			Annotations: Annotations{{Key: "rc-ann", Value: "rc"}},
		},
		Spec: &karpenterv1.NodePoolSpec{},
	})

	merged := npi.BuildReplicaNodePool(target)
	require.NotNil(t, merged)

	assert.Equal(t, "dd-pool", merged.Name)
	assert.Equal(t, metav1.TypeMeta{Kind: "NodePool", APIVersion: "karpenter.sh/v1"}, merged.TypeMeta)
	assert.Equal(t, karpenterv1.NodePoolStatus{}, merged.Status, "status must be reset")
	// Top-level metadata is completely replaced
	assert.Equal(t, map[string]string{"rc-label": "rc"}, merged.Labels)
	assert.Equal(t, map[string]string{"rc-ann": "rc"}, merged.Annotations)
	// Server-set ObjectMeta fields from the target must not leak through
	assert.Empty(t, merged.ResourceVersion, "ResourceVersion must be cleared")
	assert.Empty(t, merged.UID, "UID must be cleared")
	assert.Zero(t, merged.Generation, "Generation must be cleared")
	assert.True(t, merged.CreationTimestamp.IsZero(), "CreationTimestamp must be cleared")
	assert.Nil(t, merged.ManagedFields, "ManagedFields must be cleared")
}

func TestBuildReplicaNodePool_TargetNotMutated(t *testing.T) {
	target := targetNodePoolFixture()
	originalTargetName := target.Name
	originalTargetLabels := map[string]string{}
	for k, v := range target.Labels {
		originalTargetLabels[k] = v
	}

	npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
		Metadata: Metadata{Name: "dd-pool", Labels: Labels{{Key: "rc", Value: "v"}}},
		Spec:     &karpenterv1.NodePoolSpec{},
	})
	_ = npi.BuildReplicaNodePool(target)

	assert.Equal(t, originalTargetName, target.Name)
	assert.Equal(t, originalTargetLabels, target.Labels)
}

func TestBuildReplicaNodePool_Weight(t *testing.T) {
	tests := []struct {
		name           string
		rcWeight       *int32
		expectedWeight int32
	}{
		{
			name:           "RC weight overrides target",
			rcWeight:       func() *int32 { w := int32(42); return &w }(),
			expectedWeight: 42,
		},
		{
			name:           "RC nil falls back to target+1",
			rcWeight:       nil,
			expectedWeight: 6, // target weight 5 + 1
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     &karpenterv1.NodePoolSpec{Weight: tt.rcWeight},
			})
			merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
			require.NotNil(t, merged.Spec.Weight)
			assert.Equal(t, tt.expectedWeight, *merged.Spec.Weight)
		})
	}
}

func TestBuildReplicaNodePool_Requirements(t *testing.T) {
	rcReqs := []karpenterv1.NodeSelectorRequirementWithMinValues{
		{Key: "node.kubernetes.io/instance-type", Operator: corev1.NodeSelectorOpIn, Values: []string{"c5.large", "c5.xlarge"}},
	}
	tests := []struct {
		name     string
		rcSpec   *karpenterv1.NodePoolSpec
		expected []karpenterv1.NodeSelectorRequirementWithMinValues
	}{
		{
			name: "RC set: overrides target",
			rcSpec: &karpenterv1.NodePoolSpec{Template: karpenterv1.NodeClaimTemplate{Spec: karpenterv1.NodeClaimTemplateSpec{
				Requirements: rcReqs,
			}}},
			expected: rcReqs,
		},
		{
			name:     "RC unset: target preserved",
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: targetNodePoolFixture().Spec.Template.Spec.Requirements,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     tt.rcSpec,
			})
			merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
			assert.Equal(t, tt.expected, merged.Spec.Template.Spec.Requirements)
		})
	}
}

func TestBuildReplicaNodePool_NodeClassRef(t *testing.T) {
	tests := []struct {
		name     string
		rcSpec   *karpenterv1.NodePoolSpec
		expected string
	}{
		{
			name: "RC set: overrides target",
			rcSpec: &karpenterv1.NodePoolSpec{Template: karpenterv1.NodeClaimTemplate{Spec: karpenterv1.NodeClaimTemplateSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{Group: "karpenter.k8s.aws", Kind: "EC2NodeClass", Name: "rc-nc"},
			}}},
			expected: "rc-nc",
		},
		{
			name:     "RC unset: target preserved",
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: "target-nc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     tt.rcSpec,
			})
			merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
			assert.Equal(t, tt.expected, merged.Spec.Template.Spec.NodeClassRef.Name)
		})
	}
}

func TestBuildReplicaNodePool_Disruption(t *testing.T) {
	rcConsolidateAfter := 5 * time.Minute
	rcDisruption := karpenterv1.Disruption{
		ConsolidationPolicy: karpenterv1.ConsolidationPolicyWhenEmptyOrUnderutilized,
		ConsolidateAfter:    karpenterv1.NillableDuration{Duration: &rcConsolidateAfter},
	}
	bareTarget := &karpenterv1.NodePool{
		Spec: karpenterv1.NodePoolSpec{Template: karpenterv1.NodeClaimTemplate{Spec: karpenterv1.NodeClaimTemplateSpec{
			NodeClassRef: &karpenterv1.NodeClassReference{Name: "nc"},
			Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{},
		}}},
	}
	tests := []struct {
		name     string
		target   *karpenterv1.NodePool
		rcSpec   *karpenterv1.NodePoolSpec
		expected karpenterv1.Disruption
	}{
		{
			name:     "RC unset: target preserved",
			target:   targetNodePoolFixture(),
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: targetNodePoolFixture().Spec.Disruption,
		},
		{
			name:     "RC set: overrides target",
			target:   targetNodePoolFixture(),
			rcSpec:   &karpenterv1.NodePoolSpec{Disruption: rcDisruption},
			expected: rcDisruption,
		},
		{
			name:     "target unset, RC unset: zero value",
			target:   bareTarget,
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: karpenterv1.Disruption{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     tt.rcSpec,
			})
			merged := npi.BuildReplicaNodePool(tt.target)
			assert.Equal(t, tt.expected, merged.Spec.Disruption)
		})
	}
}

func TestBuildReplicaNodePool_ExpireAfter(t *testing.T) {
	rcExpire := 12 * time.Hour
	tests := []struct {
		name     string
		rcSpec   *karpenterv1.NodePoolSpec
		expected karpenterv1.NillableDuration
	}{
		{
			name:     "RC unset: target preserved",
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: targetNodePoolFixture().Spec.Template.Spec.ExpireAfter,
		},
		{
			name: "RC set: overrides target",
			rcSpec: &karpenterv1.NodePoolSpec{Template: karpenterv1.NodeClaimTemplate{Spec: karpenterv1.NodeClaimTemplateSpec{
				ExpireAfter: karpenterv1.NillableDuration{Duration: &rcExpire},
			}}},
			expected: karpenterv1.NillableDuration{Duration: &rcExpire},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     tt.rcSpec,
			})
			merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
			assert.Equal(t, tt.expected, merged.Spec.Template.Spec.ExpireAfter)
		})
	}
}

func TestBuildReplicaNodePool_TemplateMetadataMerged(t *testing.T) {
	npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
		Metadata: Metadata{Name: "dd-pool"},
		TemplateMetadata: &Metadata{
			Labels:      Labels{{Key: "rc-label", Value: "rc"}, {Key: "shared", Value: "rc"}},
			Annotations: Annotations{{Key: "rc-ann", Value: "rc"}},
		},
		Spec: &karpenterv1.NodePoolSpec{},
	})

	merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
	// Target keys preserved; RC keys added; shared keys overridden by RC.
	assert.Equal(t, map[string]string{
		"tmpl-label": "target",
		"rc-label":   "rc",
		"shared":     "rc",
	}, merged.Spec.Template.ObjectMeta.Labels)
	assert.Equal(t, map[string]string{
		"tmpl-ann": "target",
		"rc-ann":   "rc",
	}, merged.Spec.Template.ObjectMeta.Annotations)
}

func TestBuildReplicaNodePool_Replicas(t *testing.T) {
	rcReplicas := int64(7)
	tests := []struct {
		name     string
		rcSpec   *karpenterv1.NodePoolSpec
		expected *int64
	}{
		{
			name:     "RC set: overrides target",
			rcSpec:   &karpenterv1.NodePoolSpec{Replicas: &rcReplicas},
			expected: &rcReplicas,
		},
		{
			name:     "RC unset: nil preserved",
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     tt.rcSpec,
			})
			merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
			assert.Equal(t, tt.expected, merged.Spec.Replicas)
		})
	}
}

func TestBuildReplicaNodePool_Limits(t *testing.T) {
	rcLimits := karpenterv1.Limits{corev1.ResourceMemory: resource.MustParse("64Gi")}
	tests := []struct {
		name     string
		rcSpec   *karpenterv1.NodePoolSpec
		expected karpenterv1.Limits
	}{
		{
			name:     "RC set: overrides target",
			rcSpec:   &karpenterv1.NodePoolSpec{Limits: rcLimits},
			expected: rcLimits,
		},
		{
			name:     "RC unset: target preserved",
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: targetNodePoolFixture().Spec.Limits,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     tt.rcSpec,
			})
			merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
			assert.Equal(t, tt.expected, merged.Spec.Limits)
		})
	}
}

func TestBuildReplicaNodePool_Taints(t *testing.T) {
	rcTaints := []corev1.Taint{{Key: "rc-taint", Effect: corev1.TaintEffectPreferNoSchedule}}
	tests := []struct {
		name     string
		rcSpec   *karpenterv1.NodePoolSpec
		expected []corev1.Taint
	}{
		{
			name: "RC set: overrides target",
			rcSpec: &karpenterv1.NodePoolSpec{Template: karpenterv1.NodeClaimTemplate{Spec: karpenterv1.NodeClaimTemplateSpec{
				Taints: rcTaints,
			}}},
			expected: rcTaints,
		},
		{
			name:     "RC unset: target preserved",
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: targetNodePoolFixture().Spec.Template.Spec.Taints,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     tt.rcSpec,
			})
			merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
			assert.Equal(t, tt.expected, merged.Spec.Template.Spec.Taints)
		})
	}
}

func TestBuildReplicaNodePool_StartupTaints(t *testing.T) {
	rcStartupTaints := []corev1.Taint{{Key: "rc-startup", Effect: corev1.TaintEffectNoExecute}}
	tests := []struct {
		name     string
		rcSpec   *karpenterv1.NodePoolSpec
		expected []corev1.Taint
	}{
		{
			name: "RC set: overrides target",
			rcSpec: &karpenterv1.NodePoolSpec{Template: karpenterv1.NodeClaimTemplate{Spec: karpenterv1.NodeClaimTemplateSpec{
				StartupTaints: rcStartupTaints,
			}}},
			expected: rcStartupTaints,
		},
		{
			name:     "RC unset: target preserved",
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: targetNodePoolFixture().Spec.Template.Spec.StartupTaints,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     tt.rcSpec,
			})
			merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
			assert.Equal(t, tt.expected, merged.Spec.Template.Spec.StartupTaints)
		})
	}
}

func TestBuildReplicaNodePool_TerminationGracePeriod(t *testing.T) {
	rcGrace := metav1.Duration{Duration: time.Minute}
	tests := []struct {
		name     string
		rcSpec   *karpenterv1.NodePoolSpec
		expected *metav1.Duration
	}{
		{
			name: "RC set: overrides target",
			rcSpec: &karpenterv1.NodePoolSpec{Template: karpenterv1.NodeClaimTemplate{Spec: karpenterv1.NodeClaimTemplateSpec{
				TerminationGracePeriod: &rcGrace,
			}}},
			expected: &rcGrace,
		},
		{
			name:     "RC unset: target preserved",
			rcSpec:   &karpenterv1.NodePoolSpec{},
			expected: targetNodePoolFixture().Spec.Template.Spec.TerminationGracePeriod,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			npi := rcNodePoolFromManifest(t, &KarpenterV1NodePool{
				Metadata: Metadata{Name: "dd-pool"},
				Spec:     tt.rcSpec,
			})
			merged := npi.BuildReplicaNodePool(targetNodePoolFixture())
			assert.Equal(t, tt.expected, merged.Spec.Template.Spec.TerminationGracePeriod)
		})
	}
}
