// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestVolumeMount(t *testing.T) {
	mount := volumeMount{
		VolumeMount: corev1.VolumeMount{
			Name:      "volume",
			MountPath: "/banana",
		},
	}

	t.Run("initial volume mount", func(t *testing.T) {
		c := corev1.Container{}
		require.NoError(t, mount.mutateContainer(&c))
		require.Equal(t, []corev1.VolumeMount{mount.VolumeMount}, c.VolumeMounts, "attach a volume mount")

		require.NoError(t, mount.mutateContainer(&c))
		require.Equal(t, []corev1.VolumeMount{mount.VolumeMount}, c.VolumeMounts, "we don't re-attach it")
	})

	t.Run("we can prepend a volume mount", func(t *testing.T) {
		m2 := mount
		m2.Prepend = true

		c := corev1.Container{
			VolumeMounts: []corev1.VolumeMount{
				{
					Name: "banana",
				},
			},
		}

		require.NoError(t, m2.mutateContainer(&c))
		require.Equal(t, []corev1.VolumeMount{
			m2.VolumeMount,
			{Name: "banana"},
		}, c.VolumeMounts, "attach a volume mount")

	})
}

func TestInitContainer(t *testing.T) {
	resources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("500Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("500Mi"),
		},
	}

	c := initContainer{
		Container: corev1.Container{
			Name: "foo",
		},
		Mutators: []containerMutator{
			containerResourceRequirements{resources},
			containerSecurityContext{&corev1.SecurityContext{
				Privileged: pointer.Ptr(true),
			}},
		},
	}

	pod := common.FakePod("pod")
	require.NoError(t, c.mutatePod(pod))
	require.Equal(t, []corev1.Container{
		{
			Name:      "foo",
			Resources: resources,
			SecurityContext: &corev1.SecurityContext{
				Privileged: pointer.Ptr(true),
			},
		},
	}, pod.Spec.InitContainers)
}
