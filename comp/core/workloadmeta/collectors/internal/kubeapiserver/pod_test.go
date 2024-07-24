// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestPodParser_Parse(t *testing.T) {
	filterAnnotations := []string{"ignoreAnnotation"}

	parser, err := newPodParser(filterAnnotations)
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

	expected := []workloadmeta.Entity{&workloadmeta.KubernetesPod{
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
		Ready:                      true,
		IP:                         "127.0.0.1",
		PriorityClass:              "priorityClass",
		QOSClass:                   "Guaranteed",
	}, &workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "pods", "", "TestPod")),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Labels: map[string]string{
				"labelKey": "labelValue",
			},
			Annotations: map[string]string{
				"annotationKey": "annotationValue",
			},
		},
		GVR: &schema.GroupVersionResource{
			Group:    "",
			Resource: "pods",
			Version:  "v1",
		},
	},
	}

	assert.ElementsMatch(t, expected, parsed)

	// Assert that Annotations and Labels of all entities refer to the same address in memory
	podEntity, ok := parsed[0].(*workloadmeta.KubernetesPod)
	require.Truef(t, ok, "Failed to cast entity to *workloadmeta.KubernetesPod")

	metadataEntity, ok := parsed[1].(*workloadmeta.KubernetesMetadata)
	require.True(t, ok, "Failed to cast entity to *workloadmeta.KubernetesMetadata")

	assert.Truef(t, sameInMemory(podEntity.Annotations, metadataEntity.Annotations), "parsed annotations are duplicated in memory")
	assert.True(t, sameInMemory(podEntity.Labels, metadataEntity.Labels), "parsed labels are duplicated in memory")
}

func Test_PodsFakeKubernetesClient(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Name:   "test-pod",
		Labels: map[string]string{"test-label": "test-value"},
		UID:    types.UID("test-pod-uid"),
	}

	createResource := func(cl *fake.Clientset) error {
		_, err := cl.CoreV1().Pods(metav1.NamespaceAll).Create(context.TODO(), &corev1.Pod{ObjectMeta: objectMeta}, metav1.CreateOptions{})
		return err
	}
	expected := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesMetadata{
					EntityID: workloadmeta.EntityID{
						ID:   string(util.GenerateKubeMetadataEntityID("", "pods", "", "test-pod")),
						Kind: workloadmeta.KindKubernetesMetadata,
					},
					EntityMeta: workloadmeta.EntityMeta{
						Annotations: objectMeta.Annotations,
						Labels:      objectMeta.Labels,
					},
					GVR: &schema.GroupVersionResource{
						Group:    "",
						Version:  "v1",
						Resource: "pods",
					},
				},
			},
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesPod{
					EntityID: workloadmeta.EntityID{
						ID:   string(objectMeta.UID),
						Kind: workloadmeta.KindKubernetesPod,
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:        objectMeta.Name,
						Labels:      objectMeta.Labels,
						Annotations: objectMeta.Annotations,
					},
					Owners: []workloadmeta.KubernetesPodOwner{},
				},
			},
		},
	}
	testCollectEvent(t, createResource, newPodStore, expected)
}
