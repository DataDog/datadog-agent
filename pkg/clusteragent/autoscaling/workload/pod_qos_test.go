// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func cpuMem(cpu, mem string) corev1.ResourceList {
	rl := corev1.ResourceList{}
	if cpu != "" {
		rl[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if mem != "" {
		rl[corev1.ResourceMemory] = resource.MustParse(mem)
	}
	return rl
}

func container(name, cpuReq, memReq, cpuLim, memLim string) corev1.Container {
	return corev1.Container{
		Name: name,
		Resources: corev1.ResourceRequirements{
			Requests: cpuMem(cpuReq, memReq),
			Limits:   cpuMem(cpuLim, memLim),
		},
	}
}

func TestComputePodQoSFromSpec(t *testing.T) {
	alwaysRestart := corev1.ContainerRestartPolicyAlways

	tests := []struct {
		name string
		spec *corev1.PodSpec
		want corev1.PodQOSClass
	}{
		{
			name: "nil spec → BestEffort",
			spec: nil,
			want: corev1.PodQOSBestEffort,
		},
		{
			name: "no containers → BestEffort",
			spec: &corev1.PodSpec{},
			want: corev1.PodQOSBestEffort,
		},
		{
			name: "single container with no resources → BestEffort",
			spec: &corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
			want: corev1.PodQOSBestEffort,
		},
		{
			name: "single container with requests==limits for cpu+memory → Guaranteed",
			spec: &corev1.PodSpec{Containers: []corev1.Container{
				container("app", "500m", "1Gi", "500m", "1Gi"),
			}},
			want: corev1.PodQOSGuaranteed,
		},
		{
			name: "single container with only CPU limit (memory missing) → Burstable",
			spec: &corev1.PodSpec{Containers: []corev1.Container{
				container("app", "500m", "", "500m", ""),
			}},
			want: corev1.PodQOSBurstable,
		},
		{
			name: "single container with cpu request only → Burstable",
			spec: &corev1.PodSpec{Containers: []corev1.Container{
				container("app", "500m", "", "", ""),
			}},
			want: corev1.PodQOSBurstable,
		},
		{
			name: "request != limit → Burstable",
			spec: &corev1.PodSpec{Containers: []corev1.Container{
				container("app", "200m", "1Gi", "500m", "1Gi"),
			}},
			want: corev1.PodQOSBurstable,
		},
		{
			name: "two containers both Guaranteed-like → Guaranteed",
			spec: &corev1.PodSpec{Containers: []corev1.Container{
				container("app", "500m", "1Gi", "500m", "1Gi"),
				container("side", "100m", "256Mi", "100m", "256Mi"),
			}},
			want: corev1.PodQOSGuaranteed,
		},
		{
			name: "two containers, one missing CPU limit → Burstable",
			spec: &corev1.PodSpec{Containers: []corev1.Container{
				container("app", "500m", "1Gi", "500m", "1Gi"),
				container("side", "100m", "256Mi", "", "256Mi"),
			}},
			want: corev1.PodQOSBurstable,
		},
		{
			name: "regular init container missing limits → Burstable",
			spec: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					container("init", "100m", "128Mi", "", ""),
				},
				Containers: []corev1.Container{
					container("app", "500m", "1Gi", "500m", "1Gi"),
				},
			},
			want: corev1.PodQOSBurstable,
		},
		{
			name: "regular init container with matching requests==limits → Guaranteed",
			spec: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					container("init", "100m", "128Mi", "100m", "128Mi"),
				},
				Containers: []corev1.Container{
					container("app", "500m", "1Gi", "500m", "1Gi"),
				},
			},
			want: corev1.PodQOSGuaranteed,
		},
		{
			name: "sidecar init container missing CPU limit → Burstable",
			spec: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name:          "sidecar",
						RestartPolicy: &alwaysRestart,
						Resources: corev1.ResourceRequirements{
							Requests: cpuMem("100m", "128Mi"),
							Limits:   cpuMem("", "128Mi"),
						},
					},
				},
				Containers: []corev1.Container{
					container("app", "500m", "1Gi", "500m", "1Gi"),
				},
			},
			want: corev1.PodQOSBurstable,
		},
		{
			name: "sidecar init container with matching requests==limits → Guaranteed",
			spec: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name:          "sidecar",
						RestartPolicy: &alwaysRestart,
						Resources: corev1.ResourceRequirements{
							Requests: cpuMem("100m", "128Mi"),
							Limits:   cpuMem("100m", "128Mi"),
						},
					},
				},
				Containers: []corev1.Container{
					container("app", "500m", "1Gi", "500m", "1Gi"),
				},
			},
			want: corev1.PodQOSGuaranteed,
		},
		{
			name: "zero-quantity limits are ignored → BestEffort",
			spec: &corev1.PodSpec{Containers: []corev1.Container{
				container("app", "0", "0", "0", "0"),
			}},
			want: corev1.PodQOSBestEffort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, computePodQoSFromSpec(tt.spec))
		})
	}
}

func TestPodIsGuaranteedFromSpec(t *testing.T) {
	assert.False(t, podIsGuaranteedFromSpec(nil))
	assert.True(t, podIsGuaranteedFromSpec(&corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
		container("app", "500m", "1Gi", "500m", "1Gi"),
	}}}))
	assert.False(t, podIsGuaranteedFromSpec(&corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
		container("app", "500m", "1Gi", "", ""),
	}}}))
}

func TestPodIsGuaranteedInPlace(t *testing.T) {
	assert.False(t, podIsGuaranteedInPlace(nil))
	assert.True(t, podIsGuaranteedInPlace(&workloadmeta.KubernetesPod{QOSClass: string(corev1.PodQOSGuaranteed)}))
	assert.False(t, podIsGuaranteedInPlace(&workloadmeta.KubernetesPod{QOSClass: string(corev1.PodQOSBurstable)}))
	assert.False(t, podIsGuaranteedInPlace(&workloadmeta.KubernetesPod{QOSClass: ""}))
}
