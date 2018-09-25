// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

func TestGetSource(t *testing.T) {
	launcher := &Launcher{}
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
	serviceFoo := &service.Service{
		Type:       "docker",
		Identifier: "fooID",
	}

	source, err := launcher.getSource(pod, container, serviceFoo.Type)
	assert.Nil(t, err)
	assert.Equal(t, config.FileType, source.Config.Type)
	assert.Equal(t, "buu/fuz/foo", source.Name)
	assert.Equal(t, "/var/log/pods/baz/foo/*.log", source.Config.Path)
	assert.Equal(t, "boo", source.Config.Identifier)
	assert.Equal(t, "kubernetes", source.Config.Source)
	assert.Equal(t, "kubernetes", source.Config.Service)
	assert.IsType(t, &parser.NoopParser{}, source.Parser)
}

func TestGetSourceShouldBeOverridenByAutoDiscoveryAnnotation(t *testing.T) {
	launcher := &Launcher{}
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

	source, err := launcher.getSource(pod, container, "")
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
	launcher := &Launcher{}
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
				"ad.datadoghq.com/foo.logs": `{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}`,
			},
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{container},
		},
	}

	source, err := launcher.getSource(pod, container, "")
	assert.NotNil(t, err)
	assert.Nil(t, source)
}

func TestGetSourceAddContainerdParser(t *testing.T) {
	launcher := &Launcher{}
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
	serviceFoo := &service.Service{
		Type:       "containerd",
		Identifier: "fooID",
	}

	source, err := launcher.getSource(pod, container, serviceFoo.Type)
	assert.Nil(t, err)
	assert.Equal(t, config.FileType, source.Config.Type)
	assert.IsType(t, &parser.ContainerdFileParser{}, source.Parser)
}

func TestSearchContainer(t *testing.T) {
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
	pod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "podName",
			Namespace: "podNamespace",
			UID:       "podUID",
			Annotations: map[string]string{
				"ad.datadoghq.com/foo.logs": `{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}`,
			},
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{containerFoo, containerBar},
		},
	}

	serviceFoo := &service.Service{
		Type:       "docker",
		Identifier: "fooID",
	}
	serviceBaz := &service.Service{
		Type:       "docker",
		Identifier: "bazID",
	}

	container, _ := searchContainer(serviceFoo, pod)
	assert.Equal(t, containerFoo, container)

	_, err := searchContainer(serviceBaz, pod)
	assert.EqualError(t, err, "Container docker://bazID not found")
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
