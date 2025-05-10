// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestPodParser_Parse(t *testing.T) {
	filterAnnotations := []string{"ignoreAnnotation"}

	parser, err := NewPodParser(filterAnnotations)
	assert.NoError(t, err)

	referencePod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "TestPod",
			UID:  "uniqueIdentifier",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "deployment-hashrs",
					UID:  "ownerUID",
				},
			},
			Annotations: map[string]string{
				"annotationKey":    "annotationValue",
				"ignoreAnnotation": "ignoreValue",
			},
			Labels: map[string]string{
				"labelKey": "labelValue",
			},
		},
		Spec: corev1.PodSpec{
			PriorityClassName: "priorityClass",
			Volumes: []corev1.Volume{
				{
					Name: "pvcVol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvcName",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name: "gpuContainer1",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.Quantity{
								Format: "1",
							},
						},
					},
				},
				{
					Name: "gpuContainer2",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"gpu.intel.com/xe": resource.Quantity{
								Format: "2",
							},
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			PodIP:    "127.0.0.1",
			QOSClass: corev1.PodQOSGuaranteed,
		},
	}

	parsed := parser.Parse(&referencePod)

	expected := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "uniqueIdentifier",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "TestPod",
			Namespace: "",
			Annotations: map[string]string{
				"annotationKey": "annotationValue",
			},
			Labels: map[string]string{
				"labelKey": "labelValue",
			},
		},
		Phase: "Running",
		Owners: []workloadmeta.KubernetesPodOwner{
			{
				Kind: "ReplicaSet",
				Name: "deployment-hashrs",
				ID:   "ownerUID",
			},
		},
		PersistentVolumeClaimNames: []string{"pvcName"},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				Name: "gpuContainer1",
			},
			{
				Name: "gpuContainer2",
			},
		},
		Ready:         true,
		IP:            "127.0.0.1",
		PriorityClass: "priorityClass",
		GPUVendorList: []string{"nvidia", "intel"},
		QOSClass:      "Guaranteed",
	}

	opt := cmpopts.SortSlices(func(a, b string) bool {
		return a < b
	})
	assert.True(t,
		cmp.Equal(expected, parsed, opt),
		cmp.Diff(expected, parsed, opt),
	)
}

func TestPodParser_InstrumentationUSTs(t *testing.T) {
	parser, err := NewPodParser(nil)
	assert.NoError(t, err)

	input := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			UID:  "identifier",
			Labels: map[string]string{
				"my-env": "foo-env",
			},
			Annotations: map[string]string{
				"internal.apm.datadoghq.com/applied-target": `{
					"ddTraceConfigs": [
						{ "name": "DD_SERVICE", "value": "foo" },
						{
							"name": "DD_ENV",
							"valueFrom": {
								"fieldRef": {
									"fieldPath": "metadata.labels['my-env']"
								}
							}
						},
						{
							"name": "DD_VERSION",
							"valueFrom": {
								"fieldRef": {
									"fieldPath": "metadata.name"
								}
							}
						}
					]
				}`,
			},
		},
	}

	obj := parser.Parse(&input)
	output := obj.(*workloadmeta.KubernetesPod)

	assert.Equal(t, &workloadmeta.InstrumentationWorkloadTarget{
		Service: "foo",
		Env:     "foo-env",
		Version: "test-pod",
	}, output.EvaluatedInstrumentationWorkloadTarget)
}
