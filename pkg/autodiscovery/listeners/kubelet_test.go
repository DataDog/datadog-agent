// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	containerID          = "foobarquux"
	containerName        = "agent"
	runtimeContainerName = "k8s_datadog-agent_agent"
	podID                = "foobar"
	podName              = "datadog-agent-foobar"
	podNamespace         = "default"
)

func TestKubeletCreatePodService(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		IP: "127.0.0.1",
	}
	tests := []struct {
		name             string
		pod              *workloadmeta.KubernetesPod
		containers       []*workloadmeta.Container
		expectedServices map[string]wlmListenerSvc
	}{
		{
			name: "pod with several containers collects ports in ascending order",
			pod:  pod,
			containers: []*workloadmeta.Container{
				{
					Ports: []workloadmeta.ContainerPort{
						{
							Name: "http",
							Port: 80,
						},
					},
				},
				{
					Ports: []workloadmeta.ContainerPort{
						{
							Name: "ssh",
							Port: 22,
						},
					},
				},
			},
			expectedServices: map[string]wlmListenerSvc{
				"kubernetes_pod://foobar": {
					service: &service{
						entity:        pod,
						adIdentifiers: []string{"kubernetes_pod://foobar"},
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
						hosts: map[string]string{
							"pod": "127.0.0.1",
						},
						ready: true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener, wlm := newKubeletListener(t)

			listener.createPodService(tt.pod, tt.containers)

			wlm.assertServices(tt.expectedServices)
		})
	}
}

func TestKubeletCreateContainerService(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		IP: "127.0.0.1",
	}

	podWithAnnotations := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      podName,
			Namespace: podNamespace,
			Annotations: map[string]string{
				fmt.Sprintf("ad.datadoghq.com/%s.check.id", containerName): `customid`,
				fmt.Sprintf("ad.datadoghq.com/%s.instances", "customid"):   `[{}]`,
				fmt.Sprintf("ad.datadoghq.com/%s.check_names", "customid"): `["customcheck"]`,
				fmt.Sprintf("ad.datadoghq.com/%s.exclude", containerName):  `false`,
			},
		},
		IP: "127.0.0.1",
	}

	podWithExcludeAnnotation := podWithAnnotations.DeepCopy().(*workloadmeta.KubernetesPod)
	podWithExcludeAnnotation.Annotations[fmt.Sprintf("ad.datadoghq.com/%s.exclude", containerName)] = `true`

	podWithMetricsExcludeAnnotation := podWithAnnotations.DeepCopy().(*workloadmeta.KubernetesPod)
	podWithMetricsExcludeAnnotation.Annotations[fmt.Sprintf("ad.datadoghq.com/%s.metrics_exclude", containerName)] = `true`

	podWithLogsExcludeAnnotation := podWithAnnotations.DeepCopy().(*workloadmeta.KubernetesPod)
	podWithLogsExcludeAnnotation.Annotations[fmt.Sprintf("ad.datadoghq.com/%s.logs_exclude", containerName)] = `true`

	containerEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	containerEntityMeta := workloadmeta.EntityMeta{
		Name: runtimeContainerName,
	}

	imageWithShortname := workloadmeta.ContainerImage{
		RawName:   "gcr.io/foobar:latest",
		ShortName: "foobar",
	}

	basicImage := workloadmeta.ContainerImage{
		RawName:   "foobar",
		ShortName: "foobar",
	}

	basicContainer := &workloadmeta.Container{
		EntityID:   containerEntityID,
		EntityMeta: containerEntityMeta,
		Image:      imageWithShortname,
		State: workloadmeta.ContainerState{
			Running: true,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}

	recentlyStoppedContainer := &workloadmeta.Container{
		EntityID:   containerEntityID,
		EntityMeta: containerEntityMeta,
		Image:      basicImage,
		State: workloadmeta.ContainerState{
			Running: false,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}

	runningContainerWithFinishedAtTime := &workloadmeta.Container{
		EntityID:   containerEntityID,
		EntityMeta: containerEntityMeta,
		Image:      basicImage,
		State: workloadmeta.ContainerState{
			Running:    true,
			FinishedAt: time.Now().Add(-48 * time.Hour), // Older than default "container_exclude_stopped_age" config
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

	customIDsContainer := &workloadmeta.Container{
		EntityID:   containerEntityID,
		EntityMeta: containerEntityMeta,
		Image:      basicImage,
		State: workloadmeta.ContainerState{
			Running: true,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}

	tests := []struct {
		name             string
		pod              *workloadmeta.KubernetesPod
		podContainer     *workloadmeta.OrchestratorContainer
		container        *workloadmeta.Container
		expectedServices map[string]wlmListenerSvc
	}{
		{
			name: "basic container setup",
			pod:  pod,
			podContainer: &workloadmeta.OrchestratorContainer{
				ID:    containerID,
				Name:  containerName,
				Image: imageWithShortname,
			},
			container: basicContainer,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					parent: "kubernetes_pod://foobar",
					service: &service{
						entity: basicContainer,
						adIdentifiers: []string{
							"docker://foobarquux",
							"gcr.io/foobar:latest",
							"foobar",
						},
						hosts: map[string]string{
							"pod": "127.0.0.1",
						},
						ports: []ContainerPort{},
						extraConfig: map[string]string{
							"namespace": podNamespace,
							"pod_name":  podName,
							"pod_uid":   podID,
						},
					},
				},
			},
		},
		{
			name:      "recently stopped container excludes metrics but not logs",
			pod:       pod,
			container: recentlyStoppedContainer,
			podContainer: &workloadmeta.OrchestratorContainer{
				ID:    containerID,
				Name:  containerName,
				Image: basicImage,
			},
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					parent: "kubernetes_pod://foobar",
					service: &service{
						entity: recentlyStoppedContainer,
						adIdentifiers: []string{
							"docker://foobarquux",
							"foobar",
						},
						hosts: map[string]string{
							"pod": "127.0.0.1",
						},
						ports:           []ContainerPort{},
						metricsExcluded: true,
						extraConfig: map[string]string{
							"namespace": podNamespace,
							"pod_name":  podName,
							"pod_uid":   podID,
						},
					},
				},
			},
		},
		{
			name: "old stopped container does not get collected",
			pod:  pod,
			podContainer: &workloadmeta.OrchestratorContainer{
				ID:    containerID,
				Name:  containerName,
				Image: basicImage,
			},
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
			name: "running container with finishedAt time older than the configured threshold is collected",
			pod:  pod,
			podContainer: &workloadmeta.OrchestratorContainer{
				ID:    containerID,
				Name:  containerName,
				Image: basicImage,
			},
			container: runningContainerWithFinishedAtTime,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					parent: "kubernetes_pod://foobar",
					service: &service{
						entity: runningContainerWithFinishedAtTime,
						adIdentifiers: []string{
							"docker://foobarquux",
							"foobar",
						},
						hosts: map[string]string{
							"pod": "127.0.0.1",
						},
						ports: []ContainerPort{},
						extraConfig: map[string]string{
							"namespace": podNamespace,
							"pod_name":  podName,
							"pod_uid":   podID,
						},
					},
				},
			},
		},
		{
			name: "container with multiple ports collects them in ascending order",
			pod:  pod,
			podContainer: &workloadmeta.OrchestratorContainer{
				ID:    containerID,
				Name:  containerName,
				Image: basicImage,
			},
			container: multiplePortsContainer,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					parent: "kubernetes_pod://foobar",
					service: &service{
						entity: multiplePortsContainer,
						adIdentifiers: []string{
							"docker://foobarquux",
							"foobar",
						},
						hosts: map[string]string{
							"pod": "127.0.0.1",
						},
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
						extraConfig: map[string]string{
							"namespace": podNamespace,
							"pod_name":  podName,
							"pod_uid":   podID,
						},
					},
				},
			},
		},
		{
			name: "pod with custom check names and identifiers",
			pod:  podWithAnnotations,
			podContainer: &workloadmeta.OrchestratorContainer{
				ID:    containerID,
				Name:  containerName,
				Image: basicImage,
			},
			container: customIDsContainer,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					parent: "kubernetes_pod://foobar",
					service: &service{
						entity: customIDsContainer,
						adIdentifiers: []string{
							"customid",
							"docker://foobarquux",
							"foobar",
						},
						hosts: map[string]string{
							"pod": "127.0.0.1",
						},
						ports:      []ContainerPort{},
						checkNames: []string{"customcheck"},
						extraConfig: map[string]string{
							"namespace": podNamespace,
							"pod_name":  podName,
							"pod_uid":   podID,
						},
					},
				},
			},
		},
		{
			name: "pod with global exclude annotation does not get collected",
			pod:  podWithExcludeAnnotation,
			podContainer: &workloadmeta.OrchestratorContainer{
				ID:    containerID,
				Name:  containerName,
				Image: basicImage,
			},
			container:        customIDsContainer,
			expectedServices: map[string]wlmListenerSvc{},
		},
		{
			name: "pod with custom check names and metrics exclude annotation has metrics excluded set",
			pod:  podWithMetricsExcludeAnnotation,
			podContainer: &workloadmeta.OrchestratorContainer{
				ID:    containerID,
				Name:  containerName,
				Image: basicImage,
			},
			container: customIDsContainer,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					parent: "kubernetes_pod://foobar",
					service: &service{
						entity: customIDsContainer,
						adIdentifiers: []string{
							"customid",
							"docker://foobarquux",
							"foobar",
						},
						hosts: map[string]string{
							"pod": "127.0.0.1",
						},
						ports:      []ContainerPort{},
						checkNames: []string{"customcheck"},
						extraConfig: map[string]string{
							"namespace": podNamespace,
							"pod_name":  podName,
							"pod_uid":   podID,
						},
						metricsExcluded: true,
					},
				},
			},
		},
		{
			name: "pod with custom check names and logs exclude annotation has logs excluded set",
			pod:  podWithLogsExcludeAnnotation,
			podContainer: &workloadmeta.OrchestratorContainer{
				ID:    containerID,
				Name:  containerName,
				Image: basicImage,
			},
			container: customIDsContainer,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					parent: "kubernetes_pod://foobar",
					service: &service{
						entity: customIDsContainer,
						adIdentifiers: []string{
							"customid",
							"docker://foobarquux",
							"foobar",
						},
						hosts: map[string]string{
							"pod": "127.0.0.1",
						},
						ports:      []ContainerPort{},
						checkNames: []string{"customcheck"},
						extraConfig: map[string]string{
							"namespace": podNamespace,
							"pod_name":  podName,
							"pod_uid":   podID,
						},
						logsExcluded: true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener, wlm := newKubeletListener(t)

			listener.createContainerService(tt.pod, tt.podContainer, tt.container)

			wlm.assertServices(tt.expectedServices)
		})
	}
}

func newKubeletListener(t *testing.T) (*KubeletListener, *testWorkloadmetaListener) {
	wlm := newTestWorkloadmetaListener(t)
	filters, err := newContainerFilters()
	if err != nil {
		t.Fatalf("cannot initialize container filters: %s", err)
	}

	return &KubeletListener{workloadmetaListener: wlm, containerFilters: filters}, wlm
}
