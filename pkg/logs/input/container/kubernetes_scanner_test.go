// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package container

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestGetSource(t *testing.T) {
	scanner := KubeScanner{}
	container := kubelet.ContainerStatus{
		Name:  "foo",
		Image: "bar",
		ID:    "boo",
	}
	pod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "fuz",
			Namespace: "buu",
			UID:       "baz",
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{container},
		},
	}

	source := scanner.getSource(pod, container)
	assert.Equal(t, config.FileType, source.Config.Type)
	assert.Equal(t, "buu/fuz/foo", source.Name)
	assert.Equal(t, "/var/log/pods/baz/foo/*.log", source.Config.Path)
	assert.Equal(t, "kubernetes", source.Config.Source)
	assert.Equal(t, "kubernetes", source.Config.Service)
}
