// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// TestParseKubeletPods_ImageFromContainerSpec tests that the image from the container spec is used
func TestParseKubeletPods_ImageFromContainerSpec(t *testing.T) {
	// The container spec image should be used because it preserves the tag even when a digest is specified
	pod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid-123",
		},
		Spec: kubelet.Spec{
			Containers: []kubelet.ContainerSpec{
				{
					Name: "test-container",
					// Spec has tag + digest
					Image: "nginx:1.23.0@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
				},
			},
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{
				{
					Name: "test-container",
					ID:   "containerd://abc123",
					// Status may not have the tag (only digest)
					Image:   "nginx@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
					ImageID: "docker-pullable://nginx@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
				},
			},
		},
	}

	events := ParseKubeletPods([]*kubelet.Pod{pod}, false)

	// Find the container event
	var containerEvent *workloadmeta.CollectorEvent
	for i := range events {
		if events[i].Entity.GetID().Kind == workloadmeta.KindContainer {
			containerEvent = &events[i]
			break
		}
	}

	require.NotNil(t, containerEvent)
	container := containerEvent.Entity.(*workloadmeta.Container)

	// The image should come from the spec, which has the tag
	assert.Equal(t, "nginx:1.23.0@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0", container.Image.RawName)
	assert.Equal(t, "nginx", container.Image.Name)
	assert.Equal(t, "1.23.0", container.Image.Tag, "tag should be preserved from spec")
	assert.Equal(t, "nginx", container.Image.ShortName)
}
