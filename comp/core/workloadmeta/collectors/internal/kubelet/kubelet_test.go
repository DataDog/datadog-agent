// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"maps"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestPodParser(t *testing.T) {
	creationTimestamp := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	startTime := creationTimestamp.Add(time.Minute)

	referencePod := []*kubelet.Pod{
		{
			Metadata: kubelet.PodMetadata{
				Name:      "TestPod",
				UID:       "uniqueIdentifier",
				Namespace: "namespace",
				Owners: []kubelet.PodOwner{
					{
						Kind: "ReplicaSet",
						Name: "deployment-hashrs",
						ID:   "ownerUID",
					},
				},
				Annotations: map[string]string{
					"annotationKey": "annotationValue",
				},
				Labels: map[string]string{
					"labelKey": "labelValue",
				},
				CreationTimestamp: creationTimestamp,
			},
			Spec: kubelet.Spec{
				NodeName:          "test-node",
				HostNetwork:       true,
				PriorityClassName: "priorityClass",
				Volumes: []kubelet.VolumeSpec{
					{
						Name: "pvcVol",
						PersistentVolumeClaim: &kubelet.PersistentVolumeClaimSpec{
							ClaimName: "pvcName",
							ReadOnly:  false,
						},
					},
				},
				Tolerations: []kubelet.Toleration{
					{
						Key:               "node.kubernetes.io/not-ready",
						Operator:          "Exists",
						Effect:            "NoExecute",
						TolerationSeconds: pointer.Ptr(int64(300)),
					},
				},
				InitContainers: []kubelet.ContainerSpec{
					{
						Name:  "init-container-name",
						Image: "busybox:latest",
					},
				},
				Containers: []kubelet.ContainerSpec{
					{
						Name:  "nginx-container",
						Image: "nginx:1.25.2",
						Resources: &kubelet.ContainerResourcesSpec{
							Requests: kubelet.ResourceList{
								"nvidia.com/gpu":              resource.MustParse("1"),
								"cpu":                         resource.MustParse("100m"),
								"example.com/custom-resource": resource.MustParse("1"),
							},
							Limits: kubelet.ResourceList{
								"cpu":                         resource.MustParse("200m"),
								"example.com/custom-resource": resource.MustParse("2"),
							},
						},
						Env: []kubelet.EnvVar{
							{
								Name:  "DD_ENV",
								Value: "prod",
							},
							{
								Name:  "OTEL_SERVICE_NAME",
								Value: "$(DD_ENV)-$(DD_SERVICE)",
							},
							{
								Name:      "DD_SERVICE",
								Value:     "",
								ValueFrom: &struct{}{},
							},
						},
					},
				},
				EphemeralContainers: []kubelet.ContainerSpec{
					{
						Name:  "ephemeral-container",
						Image: "busybox:latest",
					},
				},
			},
			Status: kubelet.Status{
				Phase:     string(corev1.PodRunning),
				StartTime: startTime,
				HostIP:    "192.168.1.10",
				Reason:    "SomeReason",
				Conditions: []kubelet.Conditions{
					{
						Type:   string(corev1.PodReady),
						Status: string(corev1.ConditionTrue),
					},
				},
				PodIP:    "127.0.0.1",
				QOSClass: string(corev1.PodQOSGuaranteed),
				InitContainers: []kubelet.ContainerStatus{
					{
						Name:         "init-container-name",
						ImageID:      "sha256:abcd1234",
						Image:        "busybox:latest",
						ID:           "docker://init-containerID",
						RestartCount: 0,
					},
				},
				Containers: []kubelet.ContainerStatus{
					{
						Name:         "nginx-container",
						ImageID:      "5dbe7e1b6b9c",
						Image:        "nginx:1.25.2",
						ID:           "docker://containerID",
						Ready:        true,
						RestartCount: 1,
					},
				},
				EphemeralContainers: []kubelet.ContainerStatus{
					{
						Name:    "ephemeral-container",
						ImageID: "12345",
						Image:   "busybox:latest",
						ID:      "docker://ephemeral-container-id",
						Ready:   false,
					},
				},
			},
		},
	}

	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	events := util.ParseKubeletPods(referencePod, true, mockStore)
	parsedEntities := make([]workloadmeta.Entity, 0, len(events))
	for _, event := range events {
		parsedEntities = append(parsedEntities, event.Entity)
	}

	expectedContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "containerID",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "nginx-container",
			Labels: map[string]string{
				kubernetes.CriContainerNamespaceLabel: "namespace",
			},
		},
		Image: workloadmeta.ContainerImage{
			ID:        "5dbe7e1b6b9c",
			Name:      "nginx",
			ShortName: "nginx",
			Tag:       "1.25.2",
			RawName:   "nginx:1.25.2",
		},
		Runtime: "docker",
		Resources: workloadmeta.ContainerResources{
			GPUVendorList: []string{"nvidia"},
			CPURequest:    pointer.Ptr(10.0),
			CPULimit:      pointer.Ptr(20.0),
			RawRequests: map[string]string{
				"nvidia.com/gpu":              "1",
				"cpu":                         "100m",
				"example.com/custom-resource": "1",
			},
			RawLimits: map[string]string{
				"cpu":                         "200m",
				"example.com/custom-resource": "2",
			},
		},
		Owner: &workloadmeta.EntityID{
			Kind: "kubernetes_pod",
			ID:   "uniqueIdentifier",
		},
		Ports: []workloadmeta.ContainerPort{},
		EnvVars: map[string]string{
			"DD_ENV": "prod",
		},
		State: workloadmeta.ContainerState{
			Health: "healthy",
		},
	}

	expectedEphemeralContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "ephemeral-container-id",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "ephemeral-container",
			Labels: map[string]string{
				kubernetes.CriContainerNamespaceLabel: "namespace",
			},
		},
		Image: workloadmeta.ContainerImage{
			ID:        "12345",
			Name:      "busybox",
			ShortName: "busybox",
			Tag:       "latest",
			RawName:   "busybox:latest",
		},
		Runtime: "docker",
		Owner: &workloadmeta.EntityID{
			Kind: "kubernetes_pod",
			ID:   "uniqueIdentifier",
		},
		Ports:   []workloadmeta.ContainerPort{},
		EnvVars: map[string]string{},
		State: workloadmeta.ContainerState{
			Health: "unhealthy", // Ephemeral containers are not ready
		},
	}

	expectedInitContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "init-containerID",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "init-container-name",
			Labels: map[string]string{
				kubernetes.CriContainerNamespaceLabel: "namespace",
			},
		},
		Image: workloadmeta.ContainerImage{
			ID:        "sha256:abcd1234",
			Name:      "busybox",
			ShortName: "busybox",
			Tag:       "latest",
			RawName:   "busybox:latest",
		},
		Runtime: "docker",
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "uniqueIdentifier",
		},
		Ports:   []workloadmeta.ContainerPort{},
		EnvVars: map[string]string{},
		State: workloadmeta.ContainerState{
			Health: "unhealthy",
		},
	}

	expectedPod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "uniqueIdentifier",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "TestPod",
			Namespace: "namespace",
			Annotations: map[string]string{
				"annotationKey": "annotationValue",
			},
			Labels: map[string]string{
				"labelKey": "labelValue",
			},
		},
		Phase: "Running",
		Owners: []workloadmeta.KubernetesPodOwner{
			{
				Kind: "ReplicaSet",
				Name: "deployment-hashrs",
				ID:   "ownerUID",
			},
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				Name: "nginx-container",
				ID:   "containerID",
				Image: workloadmeta.ContainerImage{
					ID:        "5dbe7e1b6b9c",
					Name:      "nginx",
					ShortName: "nginx",
					Tag:       "1.25.2",
					RawName:   "nginx:1.25.2",
				},
			},
		},
		EphemeralContainers: []workloadmeta.OrchestratorContainer{
			{
				Name: "ephemeral-container",
				ID:   "ephemeral-container-id",
				Image: workloadmeta.ContainerImage{
					ID:        "12345",
					Name:      "busybox",
					ShortName: "busybox",
					Tag:       "latest",
					RawName:   "busybox:latest",
				},
			},
		},
		InitContainers: []workloadmeta.OrchestratorContainer{
			{
				Name: "init-container-name",
				ID:   "init-containerID",
				Image: workloadmeta.ContainerImage{
					ID:        "sha256:abcd1234",
					Name:      "busybox",
					ShortName: "busybox",
					Tag:       "latest",
					RawName:   "busybox:latest",
				},
			},
		},
		PersistentVolumeClaimNames: []string{"pvcName"},
		Ready:                      true,
		IP:                         "127.0.0.1",
		PriorityClass:              "priorityClass",
		GPUVendorList:              []string{"nvidia"},
		QOSClass:                   "Guaranteed",
		CreationTimestamp:          creationTimestamp,
		StartTime:                  &startTime,
		NodeName:                   "test-node",
		HostIP:                     "192.168.1.10",
		HostNetwork:                true,
		InitContainerStatuses: []workloadmeta.KubernetesContainerStatus{
			{
				ContainerID:  "docker://init-containerID",
				Name:         "init-container-name",
				Image:        "busybox:latest",
				ImageID:      "sha256:abcd1234",
				Ready:        false,
				RestartCount: 0,
			},
		},
		ContainerStatuses: []workloadmeta.KubernetesContainerStatus{
			{
				ContainerID:  "docker://containerID",
				Name:         "nginx-container",
				Image:        "nginx:1.25.2",
				ImageID:      "5dbe7e1b6b9c",
				Ready:        true,
				RestartCount: 1,
			},
		},
		EphemeralContainerStatuses: []workloadmeta.KubernetesContainerStatus{
			{
				ContainerID: "docker://ephemeral-container-id",
				Name:        "ephemeral-container",
				Image:       "busybox:latest",
				ImageID:     "12345",
				Ready:       false,
			},
		},
		Conditions: []workloadmeta.KubernetesPodCondition{
			{
				Type:   "Ready",
				Status: "True",
			},
		},
		Volumes: []workloadmeta.KubernetesPodVolume{
			{
				Name: "pvcVol",
				PersistentVolumeClaim: &workloadmeta.KubernetesPersistentVolumeClaim{
					ClaimName: "pvcName",
					ReadOnly:  false,
				},
			},
		},
		Tolerations: []workloadmeta.KubernetesPodToleration{
			{
				Key:               "node.kubernetes.io/not-ready",
				Operator:          "Exists",
				Effect:            "NoExecute",
				TolerationSeconds: pointer.Ptr(int64(300)),
			},
		},
		Reason: "SomeReason",
	}

	expectedEntities := []workloadmeta.Entity{
		expectedInitContainer,
		expectedContainer,
		expectedEphemeralContainer,
		expectedPod,
	}

	assert.ElementsMatch(t, expectedEntities, parsedEntities)
}

func TestEventsForExpiredEntities(t *testing.T) {
	now := time.Now()

	expiredTime := now.Add(-expireFreq - time.Second)
	nonExpiredTime := now

	tests := []struct {
		name                          string
		lastSeenPodUIDs               map[string]time.Time
		lastSeenContainerIDs          map[string]time.Time
		expectedExpiredPodUIDs        []string
		expectedExpiredContainerIDs   []string
		expectedRemainingPodUIDs      []string
		expectedRemainingContainerIDs []string
	}{
		{
			name:                          "no entities",
			lastSeenPodUIDs:               map[string]time.Time{},
			lastSeenContainerIDs:          map[string]time.Time{},
			expectedExpiredPodUIDs:        []string{},
			expectedExpiredContainerIDs:   []string{},
			expectedRemainingPodUIDs:      []string{},
			expectedRemainingContainerIDs: []string{},
		},
		{
			name: "no expired entities",
			lastSeenPodUIDs: map[string]time.Time{
				"pod1": nonExpiredTime,
			},
			lastSeenContainerIDs: map[string]time.Time{
				"docker://container1": nonExpiredTime,
			},
			expectedExpiredPodUIDs:        []string{},
			expectedExpiredContainerIDs:   []string{},
			expectedRemainingPodUIDs:      []string{"pod1"},
			expectedRemainingContainerIDs: []string{"docker://container1"},
		},
		{
			name: "expired pods only",
			lastSeenPodUIDs: map[string]time.Time{
				"expired-pod":     expiredTime,
				"not-expired-pod": nonExpiredTime,
			},
			lastSeenContainerIDs:          map[string]time.Time{},
			expectedExpiredPodUIDs:        []string{"expired-pod"},
			expectedExpiredContainerIDs:   []string{},
			expectedRemainingPodUIDs:      []string{"not-expired-pod"},
			expectedRemainingContainerIDs: []string{},
		},
		{
			name:            "expired containers only",
			lastSeenPodUIDs: map[string]time.Time{},
			lastSeenContainerIDs: map[string]time.Time{
				"docker://expired-container":     expiredTime,
				"docker://not-expired-container": nonExpiredTime,
			},
			expectedExpiredPodUIDs:        []string{},
			expectedExpiredContainerIDs:   []string{"expired-container"},
			expectedRemainingPodUIDs:      []string{},
			expectedRemainingContainerIDs: []string{"docker://not-expired-container"},
		},
		{
			name: "both expired pods and containers",
			lastSeenPodUIDs: map[string]time.Time{
				"expired-pod1": expiredTime,
				"expired-pod2": expiredTime,
			},
			lastSeenContainerIDs: map[string]time.Time{
				"docker://expired-container1": expiredTime,
				"docker://expired-container2": expiredTime,
			},
			expectedExpiredPodUIDs:        []string{"expired-pod1", "expired-pod2"},
			expectedExpiredContainerIDs:   []string{"expired-container1", "expired-container2"},
			expectedRemainingPodUIDs:      []string{},
			expectedRemainingContainerIDs: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := &collector{
				lastSeenPodUIDs:      test.lastSeenPodUIDs,
				lastSeenContainerIDs: test.lastSeenContainerIDs,
			}

			events := c.eventsForExpiredEntities(now)

			var podEvents []workloadmeta.CollectorEvent
			var containerEvents []workloadmeta.CollectorEvent
			for _, event := range events {
				if event.Entity.GetID().Kind == workloadmeta.KindKubernetesPod {
					podEvents = append(podEvents, event)
				} else if event.Entity.GetID().Kind == workloadmeta.KindContainer {
					containerEvents = append(containerEvents, event)
				}
			}

			assertUnsetEventsWithIDs(t, podEvents, test.expectedExpiredPodUIDs)
			assertUnsetEventsWithIDs(t, containerEvents, test.expectedExpiredContainerIDs)

			assert.ElementsMatch(t, slices.Collect(maps.Keys(c.lastSeenPodUIDs)), test.expectedRemainingPodUIDs)
			assert.ElementsMatch(t, slices.Collect(maps.Keys(c.lastSeenContainerIDs)), test.expectedRemainingContainerIDs)
		})
	}
}

func assertUnsetEventsWithIDs(t *testing.T, events []workloadmeta.CollectorEvent, expectedIDs []string) {
	require.Equal(t, len(events), len(expectedIDs))

	// We cannot assume order of events, so sort them to be able to compare
	sort.Strings(expectedIDs)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Entity.GetID().ID < events[j].Entity.GetID().ID
	})

	for i, event := range events {
		assert.Equal(t, workloadmeta.EventTypeUnset, event.Type)
		assert.Equal(t, workloadmeta.SourceNodeOrchestrator, event.Source)
		assert.Equal(t, expectedIDs[i], event.Entity.GetID().ID)

		// Pods contain a FinishedAt field that's not set for containers
		if pod, ok := event.Entity.(*workloadmeta.KubernetesPod); ok {
			assert.NotZero(t, pod.FinishedAt)
		}
	}
}
