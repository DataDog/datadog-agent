// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package kubernetesresourceparsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestAutoscalerParser(t *testing.T) {
	tests := []struct {
		name     string
		kind     workloadmeta.AutoscalerKind
		obj      map[string]interface{}
		expected *workloadmeta.KubernetesAutoscaler
	}{
		// Supported targets. Target kind resolution doesn't depend on the
		// autoscaler kind, so all of them are tested with HPA.
		{
			name: "HPA targeting a Deployment",
			kind: workloadmeta.AutoscalerKindHPA,
			obj: map[string]interface{}{
				"apiVersion": "autoscaling/v2",
				"kind":       "HorizontalPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"scaleTargetRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       "my-app",
					},
				},
			},
			expected: newAutoscaler("hpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindHPA, workloadmeta.AutoscalerTargetDeployment, "my-app"),
		},
		{
			name: "HPA targeting a StatefulSet",
			kind: workloadmeta.AutoscalerKindHPA,
			obj: map[string]interface{}{
				"apiVersion": "autoscaling/v2",
				"kind":       "HorizontalPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"scaleTargetRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "StatefulSet",
						"name":       "my-sts",
					},
				},
			},
			expected: newAutoscaler("hpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindHPA, workloadmeta.AutoscalerTargetStatefulSet, "my-sts"),
		},
		{
			name: "HPA targeting an Argo Rollout",
			kind: workloadmeta.AutoscalerKindHPA,
			obj: map[string]interface{}{
				"apiVersion": "autoscaling/v2",
				"kind":       "HorizontalPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"scaleTargetRef": map[string]interface{}{
						"apiVersion": "argoproj.io/v1alpha1",
						"kind":       "Rollout",
						"name":       "my-rollout",
					},
				},
			},
			expected: newAutoscaler("hpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindHPA, workloadmeta.AutoscalerTargetArgoRollout, "my-rollout"),
		},
		{
			name: "HPA owned by a KEDA ScaledObject",
			kind: workloadmeta.AutoscalerKindHPA,
			obj: map[string]interface{}{
				"apiVersion": "autoscaling/v2",
				"kind":       "HorizontalPodAutoscaler",
				"metadata": map[string]interface{}{
					"name":      "my-autoscaler",
					"namespace": "my-ns",
					"ownerReferences": []interface{}{
						map[string]interface{}{
							"apiVersion": "keda.sh/v1alpha1",
							"kind":       "ScaledObject",
						},
					},
				},
				"spec": map[string]interface{}{
					"scaleTargetRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       "my-app",
					},
				},
			},
			expected: newAutoscaler("hpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindKEDA, workloadmeta.AutoscalerTargetDeployment, "my-app"),
		},
		// Other autoscaler kinds, to check that each reads its own target ref
		// field and gets its own ID prefix.
		{
			name: "VPA targeting a Deployment",
			kind: workloadmeta.AutoscalerKindVPA,
			obj: map[string]interface{}{
				"apiVersion": "autoscaling.k8s.io/v1",
				"kind":       "VerticalPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"targetRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       "my-app",
					},
				},
			},
			expected: newAutoscaler("vpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindVPA, workloadmeta.AutoscalerTargetDeployment, "my-app"),
		},
		{
			name: "WPA targeting a Deployment",
			kind: workloadmeta.AutoscalerKindWPA,
			obj: map[string]interface{}{
				"apiVersion": "datadoghq.com/v1alpha1",
				"kind":       "WatermarkPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"scaleTargetRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       "my-app",
					},
				},
			},
			expected: newAutoscaler("wpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindWPA, workloadmeta.AutoscalerTargetDeployment, "my-app"),
		},
		{
			name: "DPA targeting a Deployment",
			kind: workloadmeta.AutoscalerKindDPA,
			obj: map[string]interface{}{
				"apiVersion": "datadoghq.com/v1alpha2",
				"kind":       "DatadogPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"targetRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       "my-app",
					},
				},
			},
			expected: newAutoscaler("dpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindDPA, workloadmeta.AutoscalerTargetDeployment, "my-app"),
		},
		// The apiVersion is optional in VPA target refs.
		{
			name: "VPA targeting a Deployment without apiVersion",
			kind: workloadmeta.AutoscalerKindVPA,
			obj: map[string]interface{}{
				"apiVersion": "autoscaling.k8s.io/v1",
				"kind":       "VerticalPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"targetRef": map[string]interface{}{
						"kind": "Deployment",
						"name": "my-app",
					},
				},
			},
			expected: newAutoscaler("vpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindVPA, workloadmeta.AutoscalerTargetDeployment, "my-app"),
		},
		// Targets that are not supported or can't be parsed. TargetKind is
		// left empty so the store filters them out.
		{
			name: "unsupported target kind",
			kind: workloadmeta.AutoscalerKindVPA,
			obj: map[string]interface{}{
				"apiVersion": "autoscaling.k8s.io/v1",
				"kind":       "VerticalPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"targetRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "DaemonSet", // DaemonSets are not supported
						"name":       "my-ds",
					},
				},
			},
			expected: newAutoscaler("vpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindVPA, "", "my-ds"),
		},
		{
			name: "supported kind in an unexpected group",
			kind: workloadmeta.AutoscalerKindHPA,
			obj: map[string]interface{}{
				"apiVersion": "autoscaling/v2",
				"kind":       "HorizontalPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"scaleTargetRef": map[string]interface{}{
						"apiVersion": "example.com/v1", // Unexpected group
						"kind":       "Deployment",
						"name":       "my-app",
					},
				},
			},
			expected: newAutoscaler("hpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindHPA, "", "my-app"),
		},
		{
			name: "missing target name",
			kind: workloadmeta.AutoscalerKindHPA,
			obj: map[string]interface{}{
				"apiVersion": "autoscaling/v2",
				"kind":       "HorizontalPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"scaleTargetRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						// Missing name
					},
				},
			},
			expected: newAutoscaler("hpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindHPA, "", ""),
		},
		{
			name: "missing spec",
			kind: workloadmeta.AutoscalerKindDPA,
			obj: map[string]interface{}{
				"apiVersion": "datadoghq.com/v1alpha2",
				"kind":       "DatadogPodAutoscaler",
				"metadata":   autoscalerMetadata(),
			},
			expected: newAutoscaler("dpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindDPA, "", ""),
		},
		{
			name: "malformed target ref",
			kind: workloadmeta.AutoscalerKindWPA,
			obj: map[string]interface{}{
				"apiVersion": "datadoghq.com/v1alpha1",
				"kind":       "WatermarkPodAutoscaler",
				"metadata":   autoscalerMetadata(),
				"spec": map[string]interface{}{
					"scaleTargetRef": "my-app", // Should be an object, not a string
				},
			},
			expected: newAutoscaler("wpa/my-ns/my-autoscaler", workloadmeta.AutoscalerKindWPA, "", ""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser, err := NewAutoscalerParser(test.kind)
			require.NoError(t, err)

			entity := parser.Parse(&unstructured.Unstructured{Object: test.obj})
			assert.Equal(t, test.expected, entity)
		})
	}
}

func TestNewAutoscalerParserUnsupportedKind(t *testing.T) {
	_, err := NewAutoscalerParser(workloadmeta.AutoscalerKindKEDA)
	assert.Error(t, err)
}

func newAutoscaler(id string, kind workloadmeta.AutoscalerKind, targetKind workloadmeta.AutoscalerTargetKind, targetName string) *workloadmeta.KubernetesAutoscaler {
	return &workloadmeta.KubernetesAutoscaler{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesAutoscaler,
			ID:   id,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "my-autoscaler",
			Namespace: "my-ns",
		},
		AutoscalerKind: kind,
		TargetKind:     targetKind,
		TargetName:     targetName,
	}
}

// autoscalerMetadata includes fields that the parser must leave out of the
// entity.
func autoscalerMetadata() map[string]interface{} {
	return map[string]interface{}{
		"name":        "my-autoscaler",
		"namespace":   "my-ns",
		"uid":         "uid-1",
		"labels":      map[string]interface{}{"app": "my-app"},
		"annotations": map[string]interface{}{"team": "my-team"},
	}
}
