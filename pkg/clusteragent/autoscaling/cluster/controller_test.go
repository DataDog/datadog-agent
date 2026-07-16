// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/model"
)

func TestIsCreatedByDatadog(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name:     "no labels are present",
			labels:   map[string]string{},
			expected: false,
		},
		{
			name: "other labels are present",
			labels: map[string]string{
				"otherLabel": "otherValue",
			},
			expected: false,
		},
		{
			name: "created label is present",
			labels: map[string]string{
				model.DatadogCreatedLabelKey: "true",
			},
			expected: true,
		},
		{
			name: "created and other label is present",
			labels: map[string]string{
				model.DatadogCreatedLabelKey: "true",
				"otherLabel":                 "otherValue",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCreatedByDatadog(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func nodeClasses(names ...string) []unstructured.Unstructured {
	ncList := make([]unstructured.Unstructured, 0, len(names))
	for _, name := range names {
		nc := unstructured.Unstructured{}
		nc.SetName(name)
		ncList = append(ncList, nc)
	}
	return ncList
}

func nodePoolWithRequirements(reqs ...karpenterv1.NodeSelectorRequirementWithMinValues) *karpenterv1.NodePool {
	return &karpenterv1.NodePool{
		Spec: karpenterv1.NodePoolSpec{
			Template: karpenterv1.NodeClaimTemplate{
				Spec: karpenterv1.NodeClaimTemplateSpec{
					Requirements: reqs,
				},
			},
		},
	}
}

func inRequirement(key string, values ...string) karpenterv1.NodeSelectorRequirementWithMinValues {
	return karpenterv1.NodeSelectorRequirementWithMinValues{
		Key:      key,
		Operator: corev1.NodeSelectorOpIn,
		Values:   values,
	}
}

func TestAttemptNodeClassMatch(t *testing.T) {
	c := &Controller{}

	tests := []struct {
		name          string
		ncList        []unstructured.Unstructured
		knp           *karpenterv1.NodePool
		expectedName  string
		expectedFound bool
	}{
		{
			name:          "unique arch match",
			ncList:        nodeClasses("linux-amd64-nodeclass", "linux-arm64-nodeclass"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "linux-amd64-nodeclass",
			expectedFound: true,
		},
		{
			name:          "arch preferred over os when both match uniquely",
			ncList:        nodeClasses("linux-amd64-nodeclass", "windows-arm64-nodeclass"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "linux-amd64-nodeclass",
			expectedFound: true,
		},
		{
			name:          "no match when no NodeClass satisfies both known os and arch, even if one dimension alone would be ambiguous",
			ncList:        nodeClasses("linux-amd64-a", "linux-amd64-b", "windows-arm64"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "windows"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedFound: false,
		},
		{
			name:          "falls back to os alone when arch is unknown",
			ncList:        nodeClasses("linux-amd64", "windows-arm64"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "windows")),
			expectedName:  "windows-arm64",
			expectedFound: true,
		},
		{
			name:          "does not fall back to arch alone when it would contradict a known os requirement",
			ncList:        nodeClasses("linux-amd64", "windows-arm64"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "windows"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedFound: false,
		},
		{
			name:          "falls back to arch alone when os is known but not encoded in any candidate name",
			ncList:        nodeClasses("ec2nodeclass-amd64", "ec2nodeclass-arm64"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "ec2nodeclass-amd64",
			expectedFound: true,
		},
		{
			name:          "no match when arch-alone and os-alone fallbacks disagree on which NodeClass to pick",
			ncList:        nodeClasses("ec2nodeclass-amd64", "ec2nodeclass-linux"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedFound: false,
		},
		{
			name:          "no match when neither os nor arch is unambiguous",
			ncList:        nodeClasses("linux-amd64-a", "linux-amd64-b"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelArchStable, "amd64")),
			expectedFound: false,
		},
		{
			name:          "no requirements means no match",
			ncList:        nodeClasses("linux-amd64-nodeclass"),
			knp:           nodePoolWithRequirements(),
			expectedFound: false,
		},
		{
			name:          "NotIn requirement is not treated as a desired value",
			ncList:        nodeClasses("linux-amd64-nodeclass", "linux-arm64-nodeclass"),
			knp:           nodePoolWithRequirements(karpenterv1.NodeSelectorRequirementWithMinValues{Key: corev1.LabelArchStable, Operator: corev1.NodeSelectorOpNotIn, Values: []string{"arm64"}}),
			expectedFound: false,
		},
		{
			name:          "multi-value In requirement with more than one distinct value is not usable for matching",
			ncList:        nodeClasses("linux-amd64-nodeclass", "windows-nodeclass"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelArchStable, "arm64", "amd64")),
			expectedFound: false,
		},
		{
			name:          "multi-value In requirement with a single distinct value (duplicates) is usable for matching",
			ncList:        nodeClasses("linux-amd64-nodeclass", "linux-arm64-nodeclass"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelArchStable, "amd64", "amd64")),
			expectedName:  "linux-amd64-nodeclass",
			expectedFound: true,
		},
		{
			name:          "combined os and arch resolve an unambiguous match that neither resolves alone",
			ncList:        nodeClasses("linux-amd64", "windows-amd64", "linux-arm64"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "linux-amd64",
			expectedFound: true,
		},
		{
			name:          "case-insensitive name segment match",
			ncList:        nodeClasses("EC2NodeClass-AMD64", "EC2NodeClass-ARM64"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "EC2NodeClass-AMD64",
			expectedFound: true,
		},
		{
			name:          "duplicate requirements for the same key are all considered",
			ncList:        nodeClasses("linux-nodeclass", "windows-nodeclass"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelOSStable, "windows")),
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, found := c.attemptNodeClassMatch(tt.ncList, tt.knp)
			assert.Equal(t, tt.expectedFound, found)
			if tt.expectedFound {
				assert.Equal(t, tt.expectedName, name)
			}
		})
	}
}

func TestUniqueNameMatch(t *testing.T) {
	tests := []struct {
		name          string
		ncList        []unstructured.Unstructured
		tokenGroups   [][]string
		expectedName  string
		expectedFound bool
	}{
		{
			name:          "exact segment match",
			ncList:        nodeClasses("team-amd64-shared"),
			tokenGroups:   [][]string{{"amd64"}},
			expectedName:  "team-amd64-shared",
			expectedFound: true,
		},
		{
			name:          "substring is not a match",
			ncList:        nodeClasses("team-amd64x-shared"),
			tokenGroups:   [][]string{{"amd64"}},
			expectedFound: false,
		},
		{
			name:          "ambiguous match across multiple NodeClasses returns false",
			ncList:        nodeClasses("linux-amd64-a", "linux-amd64-b"),
			tokenGroups:   [][]string{{"amd64"}},
			expectedFound: false,
		},
		{
			name:          "case-insensitive match",
			ncList:        nodeClasses("linux-AMD64"),
			tokenGroups:   [][]string{{"amd64"}},
			expectedName:  "linux-AMD64",
			expectedFound: true,
		},
		{
			name:          "empty groups are ignored",
			ncList:        nodeClasses("linux-amd64", "linux-arm64"),
			tokenGroups:   [][]string{{"amd64"}, nil},
			expectedName:  "linux-amd64",
			expectedFound: true,
		},
		{
			name:          "must match every non-empty group",
			ncList:        nodeClasses("linux-amd64", "windows-amd64", "linux-arm64"),
			tokenGroups:   [][]string{{"amd64"}, {"linux"}},
			expectedName:  "linux-amd64",
			expectedFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, found := uniqueNameMatch(tokenizeNames(tt.ncList), tt.tokenGroups)
			assert.Equal(t, tt.expectedFound, found)
			if tt.expectedFound {
				assert.Equal(t, tt.expectedName, name)
			}
		})
	}
}

func newUnstructuredNodeClass(gvr schema.GroupVersionResource, kind, name string) *unstructured.Unstructured {
	nc := &unstructured.Unstructured{}
	nc.SetAPIVersion(gvr.GroupVersion().String())
	nc.SetKind(kind)
	nc.SetName(name)
	return nc
}

func TestDiscoverNodeClass(t *testing.T) {
	tests := []struct {
		name            string
		ec2NodeClasses  []string
		eksNodeClasses  []string
		knp             *karpenterv1.NodePool
		expectedRefName string
		expectedErr     bool
	}{
		{
			name:            "single NodeClass is returned unambiguously",
			ec2NodeClasses:  []string{"only-nodeclass"},
			knp:             nodePoolWithRequirements(),
			expectedRefName: "only-nodeclass",
		},
		{
			name:            "ambiguity resolved via os/arch returns the matched NodeClass, not the first listed",
			ec2NodeClasses:  []string{"aaa-unrelated-nodeclass", "windows-arm64", "linux-amd64"},
			knp:             nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedRefName: "linux-amd64",
		},
		{
			name:           "unresolved ambiguity returns an error",
			ec2NodeClasses: []string{"nodeclass-a", "nodeclass-b"},
			knp:            nodePoolWithRequirements(),
			expectedErr:    true,
		},
		{
			name:            "falls back to eks NodeClass provider when no EC2NodeClass exists",
			eksNodeClasses:  []string{"only-eks-nodeclass"},
			knp:             nodePoolWithRequirements(),
			expectedRefName: "only-eks-nodeclass",
		},
		{
			name:        "no NodeClasses found from any provider returns an error",
			knp:         nodePoolWithRequirements(),
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			for _, name := range tt.ec2NodeClasses {
				objs = append(objs, newUnstructuredNodeClass(ec2NodeClassGVR, "EC2NodeClass", name))
			}
			for _, name := range tt.eksNodeClasses {
				objs = append(objs, newUnstructuredNodeClass(eksNodeClassGVR, "NodeClass", name))
			}
			fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
				map[schema.GroupVersionResource]string{
					ec2NodeClassGVR: "EC2NodeClassList",
					eksNodeClassGVR: "NodeClassList",
				},
				objs...,
			)
			c := &Controller{
				Controller: &autoscaling.Controller{Client: fakeClient},
			}

			ref, err := c.discoverNodeClass(context.Background(), tt.knp)
			if tt.expectedErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedRefName, ref.Name)
		})
	}
}
