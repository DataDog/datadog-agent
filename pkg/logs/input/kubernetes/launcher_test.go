// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"io/ioutil"
	"os"
	"path/filepath"
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
	assert.Equal(t, "/var/log/pods/buu_fuz_baz/foo/*.log", filepath.ToSlash(source.Config.Path))
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
	assert.Equal(t, "/var/log/pods/buu_fuz_baz/foo/*.log", filepath.ToSlash(source.Config.Path))
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

func TestGetSourceShouldHaveStandardServiceLabelifNoAnnotation(t *testing.T) {
	launcher := &Launcher{collectAll: true}
	container := kubelet.ContainerStatus{
		Name:  "foo",
		Image: "bar",
		ID:    "boo",
	}

	cases := []struct {
		testName string
		pod      *kubelet.Pod
		expected string
	}{
		{
			testName: "onlyServiceLabel",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "fuz",
					Namespace: "buu",
					UID:       "baz",
					Labels: map[string]string{
						"component":                  "kube-proxy",
						"tags.datadoghq.com/env":     "production",
						"tags.datadoghq.com/service": "dd-agent",
						"tags.datadoghq.com/version": "1.1.0",
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{container},
				},
			},
			expected: "dd-agent",
		},
		{
			testName: "hasAnnotation",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "fuz",
					Namespace: "buu",
					UID:       "baz",
					Labels: map[string]string{
						"component":                  "kube-proxy",
						"tags.datadoghq.com/env":     "production",
						"tags.datadoghq.com/service": "dd-agent",
						"tags.datadoghq.com/version": "1.1.0",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/foo.logs": `[{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}]`,
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{container},
				},
			},
			expected: "any_service",
		},
		{
			testName: "noServiceLabelOrAnnotation",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "fuz",
					Namespace: "buu",
					UID:       "baz",
					Labels: map[string]string{
						"component": "kube-proxy",
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{container},
				},
			},
			expected: "bar",
		},
		{
			testName: "noAnnotationServicebutHasServiceLabel",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "fuz",
					Namespace: "buu",
					UID:       "baz",
					Labels: map[string]string{
						"component":                  "kube-proxy",
						"tags.datadoghq.com/service": "dd-agent",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/foo.logs": `[{"source":"any_source","tags":["tag1","tag2"]}]`,
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{container},
				},
			},
			expected: "dd-agent",
		},
	}

	for _, tc := range cases {
		t.Run(tc.testName, func(t *testing.T) {
			source, _ := launcher.getSource(tc.pod, container)
			assert.Equal(t, tc.expected, source.Config.Service)
		})
	}
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
	containerBaz := kubelet.ContainerStatus{
		Name:  "bazName",
		Image: "bazImage",
		ID:    "containerd://bazID",
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
	podBaz := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "podName",
			Namespace: "podNamespace",
			UID:       "podUIDBaz",
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{containerBaz},
		},
	}

	source, err := launcherCollectAll.getSource(podFoo, containerFoo)
	assert.Nil(t, err)
	assert.Equal(t, "container_id://fooID", source.Config.Identifier)
	source, err = launcherCollectAll.getSource(podBar, containerBar)
	assert.Nil(t, err)
	assert.Equal(t, "container_id://barID", source.Config.Identifier)

	source, err = launcherCollectAllDisabled.getSource(podFoo, containerFoo)
	assert.Nil(t, err)
	assert.Equal(t, "container_id://fooID", source.Config.Identifier)
	source, err = launcherCollectAllDisabled.getSource(podBar, containerBar)
	assert.Equal(t, errCollectAllDisabled, err)
	assert.Nil(t, source)

	source, err = launcherCollectAll.getSource(podBaz, containerBaz)
	assert.Nil(t, err)
	assert.Equal(t, "container_id://bazID", source.Config.Identifier)
}

func TestGetPath(t *testing.T) {
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

	basePath, err := ioutil.TempDir("", "")
	defer os.RemoveAll(basePath)
	assert.Nil(t, err)

	// v1.14+ (default)
	podDirectory := "buu_fuz_baz"
	path := launcher.getPath(basePath, pod, container)
	assert.Equal(t, filepath.Join(basePath, podDirectory, "foo", "*.log"), path)

	// v1.10 - v1.13
	podDirectory = "baz"
	containerDirectory := "foo"

	err = os.MkdirAll(filepath.Join(basePath, podDirectory, containerDirectory), 0777)
	assert.Nil(t, err)

	path = launcher.getPath(basePath, pod, container)
	assert.Equal(t, filepath.Join(basePath, podDirectory, "foo", "*.log"), path)

	// v1.9
	os.RemoveAll(basePath)
	podDirectory = "baz"
	logFile := "foo_1.log"

	err = os.MkdirAll(filepath.Join(basePath, podDirectory), 0777)
	assert.Nil(t, err)

	_, err = os.Create(filepath.Join(basePath, podDirectory, logFile))
	assert.Nil(t, err)

	path = launcher.getPath(basePath, pod, container)
	assert.Equal(t, filepath.Join(basePath, podDirectory, "foo_*.log"), path)
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

func TestGetServiceLabel(t *testing.T) {
	container := kubelet.ContainerStatus{
		Name:  "foo",
		Image: "bar",
		ID:    "boo",
	}

	cases := []struct {
		testName string
		pod      *kubelet.Pod
		expected string
	}{
		{
			testName: "hasContainerServiceLabel",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "fuz",
					Namespace: "buu",
					UID:       "baz",
					Labels: map[string]string{
						"component":                      "kube-proxy",
						"tags.datadoghq.com/foo.env":     "foo-production",
						"tags.datadoghq.com/foo.service": "foo-agent",
						"tags.datadoghq.com/foo.version": "1.1.0",
						"tags.datadoghq.com/env":         "production",
						"tags.datadoghq.com/service":     "dd-agent",
						"tags.datadoghq.com/version":     "1.1.0",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/foo.logs": `[{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}]`,
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{container},
				},
			},
			expected: "foo-agent",
		},
		{
			testName: "hasOnlyStandardServiceLavel",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "fuz",
					Namespace: "buu",
					UID:       "baz",
					Labels: map[string]string{
						"component":                  "kube-proxy",
						"tags.datadoghq.com/env":     "production",
						"tags.datadoghq.com/service": "dd-agent",
						"tags.datadoghq.com/version": "1.1.0",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/foo.logs": `[{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}]`,
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{container},
				},
			},
			expected: "dd-agent",
		},
		{
			testName: "noLabels",
			pod: &kubelet.Pod{
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
			},
			expected: "",
		},
		{
			testName: "labelsExistButNoService",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "fuz",
					Namespace: "buu",
					UID:       "baz",
					Labels: map[string]string{
						"tags.datadoghq.com/env": "kube-proxy",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/foo.logs": `[{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}]`,
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{container},
				},
			},
			expected: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.testName, func(t *testing.T) {
			assert.Equal(t, tc.expected, getServiceLabel(tc.pod, container))
		})
	}

}
