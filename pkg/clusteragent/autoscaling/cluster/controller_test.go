// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
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

func nodeClassWithLabels(name string, labels map[string]string) unstructured.Unstructured {
	nc := unstructured.Unstructured{}
	nc.SetName(name)
	nc.SetLabels(labels)
	return nc
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
			name: "label match preferred over name parsing when both os and arch labels are present",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("nodeclass-a", map[string]string{corev1.LabelOSStable: "linux", corev1.LabelArchStable: "amd64"}),
				nodeClassWithLabels("nodeclass-b", map[string]string{corev1.LabelOSStable: "windows", corev1.LabelArchStable: "arm64"}),
			},
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "nodeclass-a",
			expectedFound: true,
		},
		{
			name: "label match falls back to name parsing when no candidate is labeled",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("linux-amd64-nodeclass", nil),
				nodeClassWithLabels("windows-arm64-nodeclass", nil),
			},
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "linux-amd64-nodeclass",
			expectedFound: true,
		},
		{
			name: "label match is ignored when the candidate's own name contradicts it, falls back to name parsing",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("windows-arm64-nodeclass", map[string]string{corev1.LabelOSStable: "linux", corev1.LabelArchStable: "amd64"}),
				nodeClassWithLabels("linux-amd64-nodeclass", nil),
			},
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "linux-amd64-nodeclass",
			expectedFound: true,
		},
		{
			name: "label match is case-insensitive",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("nodeclass-a", map[string]string{corev1.LabelOSStable: "Linux", corev1.LabelArchStable: "AMD64"}),
				nodeClassWithLabels("nodeclass-b", map[string]string{corev1.LabelOSStable: "windows", corev1.LabelArchStable: "arm64"}),
			},
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "nodeclass-a",
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
			name: "arch-alone fallback excludes a candidate whose own os label contradicts the requirement, rather than silently picking it",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("ec2nodeclass-amd64", map[string]string{corev1.LabelOSStable: "windows"}),
				nodeClassWithLabels("ec2nodeclass-arm64", nil),
			},
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
		{
			name:          "two requirements on the same key are intersected, not unioned",
			ncList:        nodeClasses("linux-amd64-nodeclass", "linux-arm64-nodeclass"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelArchStable, "amd64", "arm64"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "linux-amd64-nodeclass",
			expectedFound: true,
		},
		{
			name:          "requirement values are compared case-insensitively when intersecting duplicate requirements",
			ncList:        nodeClasses("linux-amd64-nodeclass", "linux-arm64-nodeclass"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelArchStable, "amd64", "AMD64")),
			expectedName:  "linux-amd64-nodeclass",
			expectedFound: true,
		},
		{
			name:          "a single empty-string requirement value is not treated as a legitimate value",
			ncList:        nodeClasses("linux-amd64-nodeclass"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "")),
			expectedFound: false,
		},
		{
			name: "label match disagreeing with name match is a genuine ambiguity, not resolved by trusting either",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("my-nodeclass", map[string]string{corev1.LabelOSStable: "linux", corev1.LabelArchStable: "amd64"}),
				nodeClassWithLabels("linux-amd64-nodeclass", nil),
			},
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedFound: false,
		},
		{
			name: "genuine label ambiguity is not overridden by a coincidentally unique name match",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("amd64-linux-nodeclass", map[string]string{corev1.LabelOSStable: "linux", corev1.LabelArchStable: "amd64"}),
				nodeClassWithLabels("random-name", map[string]string{corev1.LabelOSStable: "linux", corev1.LabelArchStable: "amd64"}),
			},
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedFound: false,
		},
		{
			name: "single-dimension fallback resolves via an arch label alone when name tokens don't mention arch",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("ec2nodeclass-1", map[string]string{corev1.LabelArchStable: "amd64"}),
				nodeClassWithLabels("ec2nodeclass-2", map[string]string{corev1.LabelArchStable: "arm64"}),
			},
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "ec2nodeclass-1",
			expectedFound: true,
		},
		{
			name:          "a name-token tie on one dimension is not silently overridden by the other dimension's unique match",
			ncList:        nodeClasses("ec2-amd64-a", "ec2-amd64-b", "ec2-linux"),
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedFound: false,
		},
		{
			name: "a clean label match is not overridden by an unrelated name-token tie among other candidates",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("ec2-a", map[string]string{corev1.LabelOSStable: "linux", corev1.LabelArchStable: "amd64"}),
				nodeClassWithLabels("linux-amd64-x", nil),
				nodeClassWithLabels("linux-amd64-y", nil),
			},
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedName:  "ec2-a",
			expectedFound: true,
		},
		{
			name: "single-dimension fallback rejects a label match whose own name contradicts that same dimension",
			ncList: []unstructured.Unstructured{
				nodeClassWithLabels("arm64-nodeclass", map[string]string{corev1.LabelArchStable: "amd64"}),
				nodeClassWithLabels("shared-pool", nil),
			},
			knp:           nodePoolWithRequirements(inRequirement(corev1.LabelOSStable, "linux"), inRequirement(corev1.LabelArchStable, "amd64")),
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, found := attemptNodeClassMatch(tt.ncList, tt.knp)
			assert.Equal(t, tt.expectedFound, found)
			if tt.expectedFound {
				assert.Equal(t, tt.expectedName, name)
			}
		})
	}
}

func TestUniqueNameMatch(t *testing.T) {
	tests := []struct {
		name              string
		ncList            []unstructured.Unstructured
		tokenGroups       [][]string
		expectedName      string
		expectedFound     bool
		expectedAmbiguous bool
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
			name:              "ambiguous match across multiple NodeClasses returns false, flagged as ambiguous rather than absent",
			ncList:            nodeClasses("linux-amd64-a", "linux-amd64-b"),
			tokenGroups:       [][]string{{"amd64"}},
			expectedFound:     false,
			expectedAmbiguous: true,
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
			name, found, ambiguous := uniqueNameMatch(tokenizeNames(tt.ncList), tt.tokenGroups)
			assert.Equal(t, tt.expectedFound, found)
			assert.Equal(t, tt.expectedAmbiguous, ambiguous)
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

func TestCheckValidNodeClass(t *testing.T) {
	tests := []struct {
		name            string
		ec2NodeClasses  []string
		existingRef     *karpenterv1.NodeClassReference
		expectedRefName string
		expectedErr     bool
	}{
		{
			name:            "no existing ref falls back to discoverNodeClass and assigns the result",
			ec2NodeClasses:  []string{"only-nodeclass"},
			expectedRefName: "only-nodeclass",
		},
		{
			name:            "existing ref that is not found falls back to discoverNodeClass and assigns the result",
			ec2NodeClasses:  []string{"only-nodeclass"},
			existingRef:     &karpenterv1.NodeClassReference{Group: ec2NodeClassGVR.Group, Kind: "EC2NodeClass", Name: "stale-nodeclass"},
			expectedRefName: "only-nodeclass",
		},
		{
			name:            "existing valid ref is kept as-is without calling discoverNodeClass",
			ec2NodeClasses:  []string{"existing-nodeclass", "other-nodeclass"},
			existingRef:     &karpenterv1.NodeClassReference{Group: ec2NodeClassGVR.Group, Kind: "EC2NodeClass", Name: "existing-nodeclass"},
			expectedRefName: "existing-nodeclass",
		},
		{
			name:        "unknown NodeClassRef group returns an error",
			existingRef: &karpenterv1.NodeClassReference{Group: "unknown.example.com", Kind: "Unknown", Name: "whatever"},
			expectedErr: true,
		},
		{
			name:        "no NodeClass discovered and no existing ref returns an error",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			for _, name := range tt.ec2NodeClasses {
				objs = append(objs, newUnstructuredNodeClass(ec2NodeClassGVR, "EC2NodeClass", name))
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

			knp := nodePoolWithRequirements()
			knp.Spec.Template.Spec.NodeClassRef = tt.existingRef

			result, err := c.checkValidNodeClass(context.Background(), knp)
			if tt.expectedErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result.Spec.Template.Spec.NodeClassRef)
			assert.Equal(t, tt.expectedRefName, result.Spec.Template.Spec.NodeClassRef.Name)
		})
	}
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
			name:            "returns eks NodeClass when EKS Auto Mode is enabled and no EC2NodeClass exists",
			eksNodeClasses:  []string{"only-eks-nodeclass"},
			knp:             nodePoolWithRequirements(),
			expectedRefName: "only-eks-nodeclass",
		},
		{
			name:            "prefers eks NodeClass over EC2NodeClass when both providers have a NodeClass",
			ec2NodeClasses:  []string{"ec2-nodeclass"},
			eksNodeClasses:  []string{"eks-nodeclass"},
			knp:             nodePoolWithRequirements(),
			expectedRefName: "eks-nodeclass",
		},
		{
			name:            "falls back to an unambiguous EC2NodeClass when the eks NodeClass tier is ambiguous",
			ec2NodeClasses:  []string{"only-ec2-nodeclass"},
			eksNodeClasses:  []string{"eks-nodeclass-a", "eks-nodeclass-b"},
			knp:             nodePoolWithRequirements(),
			expectedRefName: "only-ec2-nodeclass",
		},
		{
			name:           "returns an error when every provider tier is ambiguous",
			ec2NodeClasses: []string{"ec2-nodeclass-a", "ec2-nodeclass-b"},
			eksNodeClasses: []string{"eks-nodeclass-a", "eks-nodeclass-b"},
			knp:            nodePoolWithRequirements(),
			expectedErr:    true,
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

func TestDiscoverNodeClassAmbiguityErrorReportsEveryTier(t *testing.T) {
	objs := []runtime.Object{
		newUnstructuredNodeClass(ec2NodeClassGVR, "EC2NodeClass", "ec2-nodeclass-a"),
		newUnstructuredNodeClass(ec2NodeClassGVR, "EC2NodeClass", "ec2-nodeclass-b"),
		newUnstructuredNodeClass(eksNodeClassGVR, "NodeClass", "eks-nodeclass-a"),
		newUnstructuredNodeClass(eksNodeClassGVR, "NodeClass", "eks-nodeclass-b"),
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

	_, err := c.discoverNodeClass(context.Background(), nodePoolWithRequirements())
	require.Error(t, err)
	assert.Contains(t, err.Error(), eksNodeClassGVR.Group)
	assert.Contains(t, err.Error(), ec2NodeClassGVR.Group)
}

func TestDiscoverNodeClassReportsBothAmbiguityAndListErrorWhenBothOccur(t *testing.T) {
	objs := []runtime.Object{
		newUnstructuredNodeClass(eksNodeClassGVR, "NodeClass", "eks-nodeclass-a"),
		newUnstructuredNodeClass(eksNodeClassGVR, "NodeClass", "eks-nodeclass-b"),
	}
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			ec2NodeClassGVR: "EC2NodeClassList",
			eksNodeClassGVR: "NodeClassList",
		},
		objs...,
	)
	fakeClient.PrependReactor("list", ec2NodeClassGVR.Resource, func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("rbac denied")
	})
	c := &Controller{
		Controller: &autoscaling.Controller{Client: fakeClient},
	}

	_, err := c.discoverNodeClass(context.Background(), nodePoolWithRequirements())
	require.Error(t, err)
	assert.Contains(t, err.Error(), eksNodeClassGVR.Group)
	assert.Contains(t, err.Error(), "rbac denied")
}

func TestDiscoverNodeClassListErrorDoesNotAbortLaterTier(t *testing.T) {
	objs := []runtime.Object{
		newUnstructuredNodeClass(ec2NodeClassGVR, "EC2NodeClass", "only-ec2-nodeclass"),
	}
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			ec2NodeClassGVR: "EC2NodeClassList",
			eksNodeClassGVR: "NodeClassList",
		},
		objs...,
	)
	fakeClient.PrependReactor("list", eksNodeClassGVR.Resource, func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("rbac denied")
	})
	c := &Controller{
		Controller: &autoscaling.Controller{Client: fakeClient},
	}

	ref, err := c.discoverNodeClass(context.Background(), nodePoolWithRequirements())
	require.NoError(t, err)
	assert.Equal(t, "only-ec2-nodeclass", ref.Name)
}

func TestDiscoverNodeClassListErrorReturnedWhenNoTierSucceeds(t *testing.T) {
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			ec2NodeClassGVR: "EC2NodeClassList",
			eksNodeClassGVR: "NodeClassList",
		},
	)
	fakeClient.PrependReactor("list", eksNodeClassGVR.Resource, func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("rbac denied")
	})
	c := &Controller{
		Controller: &autoscaling.Controller{Client: fakeClient},
	}

	_, err := c.discoverNodeClass(context.Background(), nodePoolWithRequirements())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rbac denied")
}
