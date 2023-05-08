// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package listeners

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetatesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"
)

func TestCreateContainerService(t *testing.T) {
	containerEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	containerEntityMeta := workloadmeta.EntityMeta{
		Name: containerName,
	}

	basicImage := workloadmeta.ContainerImage{
		RawName:   "foobar",
		ShortName: "foobar",
	}

	basicContainer := &workloadmeta.Container{
		EntityID:   containerEntityID,
		EntityMeta: containerEntityMeta,
		Image: workloadmeta.ContainerImage{
			RawName:   "gcr.io/foobar:latest",
			ShortName: "foobar",
		},
		State: workloadmeta.ContainerState{
			Running: true,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}

	multiplePortsContainer := &workloadmeta.Container{
		EntityID:   containerEntityID,
		EntityMeta: containerEntityMeta,
		Image:      basicImage,
		Ports: []workloadmeta.ContainerPort{
			{
				Name: "http",
				Port: 80,
			},
			{
				Name: "ssh",
				Port: 22,
			},
		},
		State: workloadmeta.ContainerState{
			Running: true,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}

	runningContainerWithFinishedAtTime := &workloadmeta.Container{
		EntityID:   containerEntityID,
		EntityMeta: containerEntityMeta,
		Image: workloadmeta.ContainerImage{
			RawName:   "gcr.io/foobar:latest",
			ShortName: "foobar",
		},
		State: workloadmeta.ContainerState{
			Running:    true,
			FinishedAt: time.Now().Add(-48 * time.Hour), // Older than default "container_exclude_stopped_age" config
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}

	kubernetesContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "foo",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "foobar",
			Labels: map[string]string{
				"io.kubernetes.foo": "bar",
			},
		},
		Image: workloadmeta.ContainerImage{
			RawName:   "gcr.io/foobar:latest",
			ShortName: "foobar",
		},
		State: workloadmeta.ContainerState{
			Running: true,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}

	kubernetesExcludedContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "bar",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "barfoo",
			Labels: map[string]string{
				"io.kubernetes.foo": "bar",
			},
		},
		Image: workloadmeta.ContainerImage{
			RawName:   "gcr.io/foobar:latest",
			ShortName: "foobar",
		},
		State: workloadmeta.ContainerState{
			Running: true,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podID,
		},
	}

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      podName,
			Namespace: podNamespace,
			Annotations: map[string]string{
				fmt.Sprintf("ad.datadoghq.com/%s.exclude", kubernetesContainer.Name):         `false`,
				fmt.Sprintf("ad.datadoghq.com/%s.exclude", kubernetesExcludedContainer.Name): `true`,
			},
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:    kubernetesContainer.ID,
				Name:  kubernetesContainer.Name,
				Image: kubernetesContainer.Image,
			},
			{
				ID:    kubernetesExcludedContainer.ID,
				Name:  kubernetesExcludedContainer.Name,
				Image: kubernetesExcludedContainer.Image,
			},
		},
		IP:    "127.0.0.1",
		Ready: false,
	}

	tests := []struct {
		name             string
		container        *workloadmeta.Container
		pod              *workloadmeta.KubernetesPod
		expectedServices map[string]wlmListenerSvc
	}{
		{
			name:      "basic container setup",
			container: basicContainer,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					service: &service{
						entity: basicContainer,
						adIdentifiers: []string{
							"docker://foobarquux",
							"gcr.io/foobar",
							"foobar",
						},
						hosts: map[string]string{},
						ports: []ContainerPort{},
						ready: true,
					},
				},
			},
		},
		{
			name: "old stopped container does not get collected",
			container: &workloadmeta.Container{
				EntityID:   containerEntityID,
				EntityMeta: containerEntityMeta,
				Image:      basicImage,
				State: workloadmeta.ContainerState{
					Running:    false,
					FinishedAt: time.Now().Add(-48 * time.Hour),
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
			expectedServices: map[string]wlmListenerSvc{},
		},
		{
			// In docker, running containers can have a "finishedAt" time when
			// they have been stopped and then restarted. When that's the case,
			// we want to collect their info.
			name:      "running container with finishedAt time older than the configured threshold is collected",
			container: runningContainerWithFinishedAtTime,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					service: &service{
						entity: runningContainerWithFinishedAtTime,
						adIdentifiers: []string{
							"docker://foobarquux",
							"gcr.io/foobar",
							"foobar",
						},
						hosts: map[string]string{},
						ports: []ContainerPort{},
						ready: true,
					},
				},
			},
		},
		{
			name:      "container with multiple ports collects them in ascending order",
			container: multiplePortsContainer,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					service: &service{
						entity: multiplePortsContainer,
						adIdentifiers: []string{
							"docker://foobarquux",
							"foobar",
						},
						hosts: map[string]string{},
						ports: []ContainerPort{
							{
								Port: 22,
								Name: "ssh",
							},
							{
								Port: 80,
								Name: "http",
							},
						},
						ready: true,
					},
				},
			},
		},
		{
			name:      "running in k8s",
			container: kubernetesContainer,
			pod:       pod,
			expectedServices: map[string]wlmListenerSvc{
				"container://foo": {
					service: &service{
						entity: kubernetesContainer,
						adIdentifiers: []string{
							"docker://foo",
							"gcr.io/foobar",
							"foobar",
						},
						hosts: map[string]string{"pod": pod.IP},
						ports: []ContainerPort{},
						ready: pod.Ready,
					},
				},
			},
		},
		{
			name:             "running in k8s has excluded annotation is excluded",
			container:        kubernetesExcludedContainer,
			pod:              pod,
			expectedServices: map[string]wlmListenerSvc{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener, wlm := newContainerListener(t)
			if tt.pod != nil {
				listener.Store().(*workloadmetatesting.Store).Set(tt.pod)
			}

			listener.createContainerService(tt.container)

			wlm.assertServices(tt.expectedServices)
		})
	}
}

func TestComputeContainerServiceIDs(t *testing.T) {
	type args struct {
		entity string
		image  string
		labels map[string]string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "no labels",
			args: args{
				entity: "docker://id",
				image:  "foo/bar:latest",
				labels: map[string]string{"foo": "bar"},
			},
			want: []string{"docker://id", "foo/bar", "bar"},
		},
		{
			name: "new label",
			args: args{
				entity: "docker://id",
				image:  "foo/bar:latest",
				labels: map[string]string{"foo": "bar", "com.datadoghq.ad.check.id": "custom"},
			},
			want: []string{"custom"},
		},
		{
			name: "legacy label",
			args: args{
				entity: "docker://id",
				image:  "foo/bar:latest",
				labels: map[string]string{"foo": "bar", "com.datadoghq.sd.check.id": "custom"},
			},
			want: []string{"custom"},
		},
		{
			name: "new and legacy labels",
			args: args{
				entity: "docker://id",
				image:  "foo/bar:latest",
				labels: map[string]string{"foo": "bar", "com.datadoghq.ad.check.id": "new", "com.datadoghq.sd.check.id": "legacy"},
			},
			want: []string{"new"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, computeContainerServiceIDs(tt.args.entity, tt.args.image, tt.args.labels))
		})
	}
}

func newContainerListener(t *testing.T) (*ContainerListener, *testWorkloadmetaListener) {
	wlm := newTestWorkloadmetaListener(t)

	return &ContainerListener{workloadmetaListener: wlm}, wlm
}
