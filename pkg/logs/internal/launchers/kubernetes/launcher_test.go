// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet
// +build kubelet

package kubernetes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetatesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSource(t *testing.T) {
	launcher := getLauncher(true)
	store := workloadmetatesting.NewStore()

	store.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "baz",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "fuz",
			Namespace: "buu",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "boo",
				Name: "foo",
				Image: workloadmeta.ContainerImage{
					ShortName: "bar",
				},
			},
		},
	})

	store.Set(getBareContainer("boo", "fuz-bar-bas"))

	launcher.workloadmetaStore = store

	source, err := launcher.getSource(&service.Service{
		Type:       "docker",
		Identifier: "boo",
	})

	assert.Nil(t, err)
	assert.Equal(t, config.FileType, source.Config.Type)
	assert.Equal(t, "buu/fuz/foo", source.Name)
	assert.Equal(t, "/var/log/pods/buu_fuz_baz/foo/*.log", filepath.ToSlash(source.Config.Path))
	assert.Equal(t, "boo", source.Config.Identifier)
	assert.Equal(t, "bar", source.Config.Source)
	assert.Equal(t, "bar", source.Config.Service)
}

func TestGetSourceShouldBeOverridenByAutoDiscoveryAnnotation(t *testing.T) {
	launcher := getLauncher(true)
	store := workloadmetatesting.NewStore()

	store.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "baz",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "fuz",
			Namespace: "buu",
			Annotations: map[string]string{
				"ad.datadoghq.com/foo.logs": `[{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}]`,
			},
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "boo",
				Name: "foo",
				Image: workloadmeta.ContainerImage{
					ShortName: "bar",
				},
			},
		},
	})

	store.Set(getBareContainer("boo", "fuz-bar-baz"))

	launcher.workloadmetaStore = store

	source, err := launcher.getSource(&service.Service{
		Type:       "docker",
		Identifier: "boo",
	})

	assert.Nil(t, err)
	assert.Equal(t, config.FileType, source.Config.Type)
	assert.Equal(t, "buu/fuz/foo", source.Name)
	assert.Equal(t, "/var/log/pods/buu_fuz_baz/foo/*.log", filepath.ToSlash(source.Config.Path))
	assert.Equal(t, "boo", source.Config.Identifier)
	assert.Equal(t, "any_source", source.Config.Source)
	assert.Equal(t, "any_service", source.Config.Service)
	assert.ElementsMatch(t, source.Config.Tags, []string{"tag1", "tag2"})
}

func TestGetSourceShouldFailWithInvalidAutoDiscoveryAnnotation(t *testing.T) {
	launcher := getLauncher(true)
	store := workloadmetatesting.NewStore()

	store.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "baz",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "fuz",
			Namespace: "buu",
			Annotations: map[string]string{
				// missing [ ]
				"ad.datadoghq.com/foo.logs": `{"source":"any_source","service":"any_service","tags":["tag1","tag2"]}`,
			},
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "boo",
				Name: "foo",
				Image: workloadmeta.ContainerImage{
					ShortName: "bar",
				},
			},
		},
	})

	store.Set(getBareContainer("boo", "fuz-bar-baz"))

	launcher.workloadmetaStore = store

	source, err := launcher.getSource(&service.Service{
		Type:       "docker",
		Identifier: "boo",
	})

	assert.NotNil(t, err)
	assert.Nil(t, source)
}

func TestContainerCollectAll(t *testing.T) {
	launcherCollectAll := getLauncher(true)
	launcherCollectAllDisabled := getLauncher(false)

	store := workloadmetatesting.NewStore()

	store.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "podfoo",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "fuz",
			Namespace: "buu",
			Annotations: map[string]string{
				"ad.datadoghq.com/foo.logs": `[{"source":"any_source","service":"any_service"}]`,
			},
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "foo",
				Name: "foo",
			},
			{
				ID:   "bar",
				Name: "bar",
			},
		},
	})

	store.Set(getBareContainer("foo", "foo"))
	store.Set(getBareContainer("bar", "bar"))

	launcherCollectAll.workloadmetaStore = store
	launcherCollectAllDisabled.workloadmetaStore = store

	source, err := launcherCollectAll.getSource(&service.Service{
		Type:       "docker",
		Identifier: "foo",
	})
	assert.Nil(t, err)
	assert.Equal(t, "foo", source.Config.Identifier)

	source, err = launcherCollectAll.getSource(&service.Service{
		Type:       "docker",
		Identifier: "bar",
	})
	assert.Nil(t, err)
	assert.Equal(t, "bar", source.Config.Identifier)

	source, err = launcherCollectAllDisabled.getSource(&service.Service{
		Type:       "docker",
		Identifier: "foo",
	})
	assert.Nil(t, err)
	assert.Equal(t, "foo", source.Config.Identifier)

	source, err = launcherCollectAllDisabled.getSource(&service.Service{
		Type:       "docker",
		Identifier: "bar",
	})
	assert.Equal(t, errCollectAllDisabled, err)
	assert.Nil(t, source)
}

func TestGetPath(t *testing.T) {
	launcher := getLauncher(true)
	container := "foo"
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			ID: "baz",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "fuz",
			Namespace: "buu",
		},
	}

	basePath := t.TempDir()

	// v1.14+ (default)
	podDirectory := "buu_fuz_baz"
	path := launcher.getPath(basePath, pod, container)
	assert.Equal(t, filepath.Join(basePath, podDirectory, "foo", "*.log"), path)

	// v1.10 - v1.13
	podDirectory = "baz"
	containerDirectory := "foo"

	err := os.MkdirAll(filepath.Join(basePath, podDirectory, containerDirectory), 0777)
	assert.Nil(t, err)

	path = launcher.getPath(basePath, pod, container)
	assert.Equal(t, filepath.Join(basePath, podDirectory, "foo", "*.log"), path)

	// v1.9
	os.RemoveAll(basePath)
	podDirectory = "baz"
	logFile := "foo_1.log"

	err = os.MkdirAll(filepath.Join(basePath, podDirectory), 0777)
	assert.Nil(t, err)

	f, err := os.Create(filepath.Join(basePath, podDirectory, logFile))
	assert.Nil(t, err)
	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})

	path = launcher.getPath(basePath, pod, container)
	assert.Equal(t, filepath.Join(basePath, podDirectory, "foo_*.log"), path)
}

func TestGetSourceServiceNameOrder(t *testing.T) {
	tests := []struct {
		name            string
		sFunc           func(string, string) string
		pod             *workloadmeta.KubernetesPod
		container       *workloadmeta.Container
		wantServiceName string
		wantSourceName  string
		wantErr         bool
	}{
		{
			name:  "log config",
			sFunc: func(n, e string) string { return "stdServiceName" },
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "podUIDFoo",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "podName",
					Namespace: "podNamespace",
					Annotations: map[string]string{
						"ad.datadoghq.com/fooName.logs": `[{"source":"foo","service":"annotServiceName"}]`,
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "fooID",
						Name: "fooName",
						Image: workloadmeta.ContainerImage{
							ShortName: "fooImage",
						},
					},
				},
			},
			container:       getBareContainer("fooID", "fooName"),
			wantServiceName: "annotServiceName",
			wantSourceName:  "foo",
			wantErr:         false,
		},
		{
			name:  "standard tags",
			sFunc: func(n, e string) string { return "stdServiceName" },
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "podUIDFoo",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "podName",
					Namespace: "podNamespace",
					Annotations: map[string]string{
						"ad.datadoghq.com/fooName.logs": `[{"source":"foo"}]`,
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "fooID",
						Name: "fooName",
						Image: workloadmeta.ContainerImage{
							ShortName: "fooImage",
						},
					},
				},
			},
			container:       getBareContainer("fooID", "fooName"),
			wantServiceName: "stdServiceName",
			wantSourceName:  "foo",
			wantErr:         false,
		},
		{
			name:  "standard tags, undefined source, use image as source",
			sFunc: func(n, e string) string { return "stdServiceName" },
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "podUIDFoo",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "podName",
					Namespace: "podNamespace",
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "fooID",
						Name: "fooName",
						Image: workloadmeta.ContainerImage{
							ShortName: "fooImage",
						},
					},
				},
			},
			container:       getBareContainer("fooID", "fooName"),
			wantServiceName: "stdServiceName",
			wantSourceName:  "fooImage",
			wantErr:         false,
		},
		{
			name:  "image name",
			sFunc: func(n, e string) string { return "" },
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "podUIDFoo",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "podName",
					Namespace: "podNamespace",
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "fooID",
						Name: "fooName",
						Image: workloadmeta.ContainerImage{
							ShortName: "fooImage",
						},
					},
				},
			},
			container:       getBareContainer("fooID", "fooName"),
			wantServiceName: "fooImage",
			wantSourceName:  "fooImage",
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := workloadmetatesting.NewStore()
			store.Set(tt.container)
			store.Set(tt.pod)
			l := &Launcher{
				collectAll:        true,
				serviceNameFunc:   tt.sFunc,
				workloadmetaStore: store,
			}

			got, err := l.getSource(&service.Service{
				Type:       string(tt.container.Runtime),
				Identifier: tt.container.ID,
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("Launcher.getSource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantServiceName, got.Config.Service)
			assert.Equal(t, tt.wantSourceName, got.Config.Source)
		})
	}
}

func getBareContainer(id, name string) *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   id,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: name,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}
}

func getLauncher(collectAll bool) *Launcher {
	return &Launcher{
		collectAll:      collectAll,
		serviceNameFunc: func(string, string) string { return "" },
	}
}
