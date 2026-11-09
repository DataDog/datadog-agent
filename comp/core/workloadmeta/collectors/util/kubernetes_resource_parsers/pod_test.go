// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubernetesresourceparsers

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestPodParser_Parse(t *testing.T) {
	filterAnnotations := []string{"ignoreAnnotation"}

	parser, err := NewPodParser(filterAnnotations)
	require.NoError(t, err)

	creationTime := metav1.NewTime(time.Date(2026, 10, 19, 19, 0, 0, 0, time.UTC))
	readyTime := metav1.NewTime(time.Date(2026, 10, 19, 19, 1, 0, 0, time.UTC))
	resizeTime := metav1.NewTime(time.Date(2026, 10, 19, 19, 28, 50, 0, time.UTC))
	deletionTime := metav1.NewTime(time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC))
	runtimeClass := "runtimeClass"

	referencePod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "TestPod",
			Namespace:         "TestNamespace",
			UID:               "uniqueIdentifier",
			CreationTimestamp: creationTime,
			DeletionTimestamp: &deletionTime,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "deployment-hashrs",
					UID:        "ownerUID",
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
			NodeName:          "nodeName",
			PriorityClassName: "priorityClass",
			RuntimeClassName:  &runtimeClass,
			Volumes: []corev1.Volume{
				{
					Name: "pvcVol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvcName",
						},
					},
				},
				{
					Name: "configMapVol",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "configMapName"},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "gpuContainer1",
					Image: "registry.example.com/team/app:1.2.3",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("250m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.Quantity{
								Format: "1",
							},
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
					ResizePolicy: []corev1.ContainerResizePolicy{
						{ResourceName: corev1.ResourceCPU, RestartPolicy: corev1.NotRequired},
						{ResourceName: corev1.ResourceMemory, RestartPolicy: corev1.RestartContainer},
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
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: readyTime,
				},
				{
					Type:               corev1.PodResizePending,
					Status:             corev1.ConditionTrue,
					Reason:             corev1.PodReasonInfeasible,
					LastTransitionTime: resizeTime,
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
			Namespace: "TestNamespace",
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
				Kind:  "ReplicaSet",
				Name:  "deployment-hashrs",
				ID:    "ownerUID",
				Group: "apps",
			},
		},
		PersistentVolumeClaimNames: []string{"pvcName"},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				Name: "gpuContainer1",
				Image: workloadmeta.ContainerImage{
					RawName:   "registry.example.com/team/app:1.2.3",
					Name:      "registry.example.com/team/app",
					Registry:  "registry.example.com",
					ShortName: "app",
					Tag:       "1.2.3",
				},
				Resources: workloadmeta.ContainerResources{
					CPURequest:    pointer.Ptr(25.0),
					CPULimit:      pointer.Ptr(50.0),
					MemoryRequest: pointer.Ptr[uint64](128 * 1024 * 1024),
					MemoryLimit:   pointer.Ptr[uint64](256 * 1024 * 1024),
				},
				ResizePolicy: workloadmeta.ContainerResizePolicy{
					CPURestartPolicy:    "NotRequired",
					MemoryRestartPolicy: "RestartContainer",
				},
			},
			{
				Name: "gpuContainer2",
			},
		},
		Conditions: []workloadmeta.KubernetesPodCondition{
			{
				Type:               "PodResizePending",
				Status:             "True",
				Reason:             "Infeasible",
				LastTransitionTime: resizeTime.Time,
			},
		},
		Ready:             true,
		IP:                "127.0.0.1",
		PriorityClass:     "priorityClass",
		GPUVendorList:     []string{"nvidia", "intel"},
		QOSClass:          "Guaranteed",
		RuntimeClass:      "runtimeClass",
		CreationTimestamp: creationTime.Time,
		ReadyTimestamp:    &readyTime.Time,
		DeletionTimestamp: &deletionTime.Time,
		NodeName:          "nodeName",
	}

	opt := cmpopts.SortSlices(func(a, b string) bool {
		return a < b
	})
	assert.True(t,
		cmp.Equal(expected, parsed, opt),
		cmp.Diff(expected, parsed, opt),
	)
}

func TestPodParser_InvalidAnnotationsExclude(t *testing.T) {
	parser, err := NewPodParser([]string{"excludedAnnotation", "("})
	assert.Error(t, err)
	require.NotNil(t, parser)

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"excludedAnnotation": "excludedValue",
				"keptAnnotation":     "keptValue",
			},
		},
	}

	parsed := parser.Parse(&pod).(*workloadmeta.KubernetesPod)
	assert.Equal(t, map[string]string{"keptAnnotation": "keptValue"}, parsed.Annotations)
}

// Exercise the API-server pod parser rather than constructing image-bearing workloadmeta pods.
func TestPodParserRayInferenceTags(t *testing.T) {
	for _, tt := range []struct {
		name          string
		containerName string
		image         string
		imageName     string
		nodeType      string
		group         string
		wantComponent string
		sidecarImage  string
		serveFalse    bool
	}{
		{name: "head", containerName: "ray-head", image: "rayproject/ray-llm:latest", imageName: "rayproject/ray-llm", nodeType: "head", group: "headgroup", wantComponent: "head"},
		{name: "private worker", containerName: "ray-worker", image: "example.com/team/ray-llm-service:vllm-stable", imageName: "example.com/team/ray-llm-service", nodeType: "worker", group: "decode", wantComponent: "decode"},
		{name: "vllm digest", containerName: "ray-worker", image: "example.com/team/vllm@sha256:0123456789abcdef", imageName: "example.com/team/vllm", nodeType: "worker", wantComponent: "worker"},
		{name: "generic Ray", containerName: "ray-worker", image: "rayproject/ray:latest", imageName: "rayproject/ray", nodeType: "worker"},
		{name: "LLM tag only", containerName: "ray-worker", image: "example.com/team/ray:vllm-stable", imageName: "example.com/team/ray", nodeType: "worker"},
		{name: "LLM namespace only", containerName: "ray-worker", image: "example.com/vllm/ray:latest", imageName: "example.com/vllm/ray", nodeType: "worker"},
		{name: "LLM registry only", containerName: "ray-worker", image: "ray-llm.example.com/team/ray:latest", imageName: "ray-llm.example.com/team/ray", nodeType: "worker"},
		{name: "sidecar only", containerName: "ray-worker", image: "rayproject/ray:latest", imageName: "rayproject/ray", sidecarImage: "vllm/vllm-openai:latest", nodeType: "worker"},
		{name: "custom worker", containerName: "llm", image: "rayproject/ray-llm:2.56.0-py312-cu130", imageName: "rayproject/ray-llm", nodeType: "worker", group: "gpu-group", wantComponent: "gpu-group"},
		{name: "head excluded from traffic", containerName: "ray-head", image: "rayproject/ray-llm:latest", imageName: "rayproject/ray-llm", nodeType: "head", wantComponent: "head", serveFalse: true},
		{name: "missing image", containerName: "ray-worker", nodeType: "worker"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ray-pod", UID: "ray-pod-uid",
					Labels: map[string]string{
						"ray.io/cluster": "nlq-mixed-nqlnx", "ray.io/serve": "true",
						"ray.io/node-type": tt.nodeType, "ray.io/group": tt.group,
					},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: tt.containerName, Image: tt.image}}},
			}
			if tt.serveFalse {
				pod.Labels["ray.io/serve"] = "false"
			}
			if tt.sidecarImage != "" {
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: "sidecar", Image: tt.sidecarImage})
			}
			parser, err := NewPodParser(nil)
			require.NoError(t, err)
			parsedPod := parser.Parse(pod).(*workloadmeta.KubernetesPod)
			require.Len(t, parsedPod.Containers, len(pod.Spec.Containers))
			assert.Equal(t, tt.imageName, parsedPod.Containers[0].Image.Name)
			extractor := collectors.NewPodTagExtractor(config.NewMock(t), nil)
			var got []string
			for _, tag := range extractor.Extract(parsedPod, taggertypes.LowCardinality) {
				if strings.HasPrefix(tag, "inference_workload:") || strings.HasPrefix(tag, "inference_workload_component:") {
					got = append(got, tag)
				}
			}
			var want []string
			if tt.wantComponent != "" {
				want = []string{"inference_workload:nlq-mixed", "inference_workload_component:" + tt.wantComponent}
			}
			assert.ElementsMatch(t, want, got)
		})
	}
}

func TestResizeConditionsFromPodConditions(t *testing.T) {
	transition := metav1.NewTime(time.Date(2026, 10, 19, 19, 28, 50, 0, time.UTC))

	tests := []struct {
		name       string
		conditions []corev1.PodCondition
		want       []workloadmeta.KubernetesPodCondition
	}{
		{
			name:       "no conditions",
			conditions: nil,
			want:       nil,
		},
		{
			name: "non-resize conditions are dropped",
			conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
			},
			want: nil,
		},
		{
			name: "pending and in-progress are kept, others dropped",
			conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				{Type: corev1.PodResizePending, Status: corev1.ConditionTrue, Reason: corev1.PodReasonInfeasible, LastTransitionTime: transition},
				{Type: corev1.PodResizeInProgress, Status: corev1.ConditionTrue, Reason: corev1.PodReasonError},
			},
			want: []workloadmeta.KubernetesPodCondition{
				{Type: "PodResizePending", Status: "True", Reason: "Infeasible", LastTransitionTime: transition.Time},
				{Type: "PodResizeInProgress", Status: "True", Reason: "Error"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resizeConditionsFromPodConditions(tt.conditions))
		})
	}
}

func TestResizePolicyFromContainerResizePolicy(t *testing.T) {
	rule := func(r corev1.ResourceName, p corev1.ResourceResizeRestartPolicy) corev1.ContainerResizePolicy {
		return corev1.ContainerResizePolicy{ResourceName: r, RestartPolicy: p}
	}

	tests := []struct {
		name  string
		rules []corev1.ContainerResizePolicy
		want  workloadmeta.ContainerResizePolicy
	}{
		{
			name:  "no rules",
			rules: nil,
			want:  workloadmeta.ContainerResizePolicy{},
		},
		{
			name:  "cpu only",
			rules: []corev1.ContainerResizePolicy{rule(corev1.ResourceCPU, corev1.NotRequired)},
			want:  workloadmeta.ContainerResizePolicy{CPURestartPolicy: string(corev1.NotRequired)},
		},
		{
			name:  "memory only",
			rules: []corev1.ContainerResizePolicy{rule(corev1.ResourceMemory, corev1.RestartContainer)},
			want:  workloadmeta.ContainerResizePolicy{MemoryRestartPolicy: string(corev1.RestartContainer)},
		},
		{
			name: "both resources",
			rules: []corev1.ContainerResizePolicy{
				rule(corev1.ResourceCPU, corev1.NotRequired),
				rule(corev1.ResourceMemory, corev1.RestartContainer),
			},
			want: workloadmeta.ContainerResizePolicy{
				CPURestartPolicy:    string(corev1.NotRequired),
				MemoryRestartPolicy: string(corev1.RestartContainer),
			},
		},
		{
			name:  "unsupported resource ignored",
			rules: []corev1.ContainerResizePolicy{rule(corev1.ResourceName("ephemeral-storage"), corev1.RestartContainer)},
			want:  workloadmeta.ContainerResizePolicy{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resizePolicyFromContainerResizePolicy(tc.rules))
		})
	}
}
