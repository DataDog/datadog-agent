// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestGetSource(t *testing.T) {
	launcher := &Launcher{collectAll: true}
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

	source, err := launcher.getSource(pod, container)
	assert.Nil(t, err)
	assert.Equal(t, config.FileType, source.Config.Type)
	assert.Equal(t, "buu/fuz/foo", source.Name)
	assert.Equal(t, "/var/log/pods/baz/foo/*.log", source.Config.Path)
	assert.Equal(t, "boo", source.Config.Identifier)
	assert.Equal(t, "bar", source.Config.Source)
	assert.Equal(t, "bar", source.Config.Service)
}

func TestGetSourceShouldBeOverridenByAutoDiscoveryAnnotation(t *testing.T) {
	launcher := &Launcher{collectAll: true}
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
			Annotations: map[string]string{
				"ad.datadoghq.com/foo.logs": `[{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}]`,
			},
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{container},
		},
	}

	source, err := launcher.getSource(pod, container)
	assert.Nil(t, err)
	assert.Equal(t, config.FileType, source.Config.Type)
	assert.Equal(t, "buu/fuz/foo", source.Name)
	assert.Equal(t, "/var/log/pods/baz/foo/*.log", source.Config.Path)
	assert.Equal(t, "boo", source.Config.Identifier)
	assert.Equal(t, "any_source", source.Config.Source)
	assert.Equal(t, "any_service", source.Config.Service)
	assert.True(t, contains(source.Config.Tags, "tag1", "tag2"))
}

func TestGetSourceShouldFailWithInvalidAutoDiscoveryAnnotation(t *testing.T) {
	launcher := &Launcher{collectAll: true}
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
			Annotations: map[string]string{
				// missing [ ]
				"ad.datadoghq.com/foo.logs": `{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}`,
			},
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{container},
		},
	}

	source, err := launcher.getSource(pod, container)
	assert.NotNil(t, err)
	assert.Nil(t, source)
}

func TestGetSourceAddContainerdParser(t *testing.T) {
	launcher := &Launcher{collectAll: true}
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

	source, err := launcher.getSource(pod, container)
	assert.Nil(t, err)
	assert.Equal(t, config.FileType, source.Config.Type)
}

func TestContainerCollectAll(t *testing.T) {
	launcherCollectAll := &Launcher{collectAll: true}
	launcherCollectAllDisabled := &Launcher{collectAll: false}
	containerFoo := kubelet.ContainerStatus{
		Name:  "fooName",
		Image: "fooImage",
		ID:    "docker://fooID",
	}
	containerBar := kubelet.ContainerStatus{
		Name:  "barName",
		Image: "barImage",
		ID:    "docker://barID",
	}
	podFoo := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "podName",
			Namespace: "podNamespace",
			UID:       "podUIDFoo",
			Annotations: map[string]string{
				"ad.datadoghq.com/fooName.logs": `[{"source":"any_source","service":"any_service"}]`,
			},
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{containerFoo, containerBar},
		},
	}
	podBar := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "podName",
			Namespace: "podNamespace",
			UID:       "podUIDBarr",
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{containerFoo, containerBar},
		},
	}

	source, err := launcherCollectAll.getSource(podFoo, containerFoo)
	assert.Nil(t, err)
	assert.Equal(t, "docker://fooID", source.Config.Identifier)
	source, err = launcherCollectAll.getSource(podBar, containerBar)
	assert.Nil(t, err)
	assert.Equal(t, "docker://barID", source.Config.Identifier)

	source, err = launcherCollectAllDisabled.getSource(podFoo, containerFoo)
	assert.Nil(t, err)
	assert.Equal(t, "docker://fooID", source.Config.Identifier)
	source, err = launcherCollectAllDisabled.getSource(podBar, containerBar)
	assert.Equal(t, collectAllDisabledError, err)
	assert.Nil(t, source)
}

// contains returns true if the list contains all the items.
func contains(list []string, items ...string) bool {
	m := make(map[string]struct{}, len(items))
	for _, item := range items {
		m[item] = struct{}{}
	}
	for _, elt := range list {
		if _, exists := m[elt]; !exists {
			return false
		}
	}
	return true
}
