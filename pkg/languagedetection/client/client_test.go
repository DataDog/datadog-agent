// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
)

type MockDCAClient struct {
	payload []*pbgo.ParentLanguageAnnotationRequest
	doneCh  chan struct{}
}

func (m *MockDCAClient) PostLanguageMetadata(ctx context.Context, request *pbgo.ParentLanguageAnnotationRequest) error {
	m.payload = append(m.payload, request)
	m.doneCh <- struct{}{}
	return nil
}

func TestClientFlush(t *testing.T) {
	doneCh := make(chan struct{})
	mockDCAClient := &MockDCAClient{doneCh: doneCh}
	client := NewClient(context.Background(), nil, nil, mockDCAClient)

	container := &containerInfo{
		languages: map[string]*languagesSet{
			"java-cont": {
				languages: map[string]struct{}{
					"java": {},
				},
			},
		},
	}

	initContainer := &containerInfo{
		languages: map[string]*languagesSet{
			"go-cont": {
				languages: map[string]struct{}{
					"go": {},
				},
			},
		},
	}

	podInfo := &podInfo{
		namespace:         "default",
		containerInfo:     container,
		initContainerInfo: initContainer,
		ownerRef: &workloadmeta.KubernetesPodOwner{
			Name: "dummyrs",
			Kind: "replicaset",
			ID:   "dummyid",
		},
	}
	podName := "nginx"
	client.currentBatch.podInfo[podName] = podInfo
	client.flush()
	<-doneCh
	assert.Equal(t, []*pbgo.ParentLanguageAnnotationRequest{
		{
			PodDetails: []*pbgo.PodLanguageDetails{
				{
					Name:                 podName,
					Namespace:            podInfo.namespace,
					InitContainerDetails: podInfo.initContainerInfo.toProto(),
					ContainerDetails:     podInfo.containerInfo.toProto(),
					Ownerref: &pbgo.KubeOwnerInfo{
						Name: "dummyrs",
						Kind: "replicaset",
						Id:   "dummyid",
					},
				},
			},
		},
	}, mockDCAClient.payload)
}

func TestClientProcessEvent(t *testing.T) {
	ctx := context.Background()
	mockConfig := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	mockConfig.Set("language_detection.client_period", time.Second)

	mockStore := workloadmeta.NewMockStore()
	client := NewClient(ctx, mockConfig, mockStore, &MockDCAClient{})

	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "nginx-cont-id",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "nginx-cont-name",
		},
		Owner: &workloadmeta.EntityID{
			ID:   "nginx-pod-id",
			Kind: workloadmeta.KindKubernetesPod,
		},
	}

	initContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "init-nginx-cont-id",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "nginx-cont-name",
		},
		Owner: &workloadmeta.EntityID{
			ID:   "nginx-pod-id",
			Kind: workloadmeta.KindKubernetesPod,
		},
	}

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			ID:   "nginx-pod-id",
			Kind: workloadmeta.KindKubernetesPod,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "nginx-pod-name",
			Namespace: "nginx-pod-namespace",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "nginx-cont-id",
				Name: "nginx-cont-name",
			},
		},
		InitContainers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "init-nginx-cont-id",
				Name: "nginx-cont-name",
			},
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{
				ID:   "nginx-replicaset-id",
				Name: "nginx-replicaset-name",
				Kind: "replicaset",
			},
		},
	}

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Language: &languagemodels.Language{
			Name: "java",
		},
		ContainerId: "nginx-cont-id",
	}

	initProcess := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Language: &languagemodels.Language{
			Name: "go",
		},
		ContainerId: "init-nginx-cont-id",
	}

	eventBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Entity: initProcess,
				Type:   workloadmeta.EventTypeSet,
			},
			{
				Entity: process,
				Type:   workloadmeta.EventTypeSet,
			},
		},
		Ch: make(chan struct{}),
	}

	collectorEvents := []workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: container,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: initContainer,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: initProcess,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: process,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: pod,
		},
	}

	mockStore.Notify(collectorEvents)

	client.processEvent(eventBundle)

	assert.NotEmpty(t, client.currentBatch.podInfo)
	assert.Equal(t,
		map[string]*podInfo{
			"nginx-pod-name": {
				namespace: "nginx-pod-namespace",
				containerInfo: &containerInfo{
					map[string]*languagesSet{
						"nginx-cont-name": {
							languages: map[string]struct{}{"java": {}},
						},
					},
				},
				initContainerInfo: &containerInfo{
					map[string]*languagesSet{
						"nginx-cont-name": {
							languages: map[string]struct{}{"go": {}},
						},
					},
				},
				ownerRef: &workloadmeta.KubernetesPodOwner{
					ID:   "nginx-replicaset-id",
					Name: "nginx-replicaset-name",
					Kind: "replicaset",
				},
			},
		},
		client.currentBatch.podInfo,
	)
}

func TestGetContainerInfoFromPod(t *testing.T) {
	tests := []struct {
		name            string
		containerID     string
		pod             *workloadmeta.KubernetesPod
		expectedName    string
		isInitContainer bool
		found           bool
	}{
		{
			name:        "not found",
			containerID: "cid",
			pod: &workloadmeta.KubernetesPod{
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "java-id",
						Name: "java-name",
					},
				},
				InitContainers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "java-id-2",
						Name: "java-name",
					},
				},
			},
			expectedName:    "",
			isInitContainer: false,
			found:           false,
		},
		{
			name:        "init container",
			containerID: "java-id-2",
			pod: &workloadmeta.KubernetesPod{
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "java-id",
						Name: "java-name",
					},
				},
				InitContainers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "java-id-2",
						Name: "java-name-2",
					},
				},
			},
			expectedName:    "java-name-2",
			isInitContainer: true,
			found:           true,
		},
		{
			name:        "normal container",
			containerID: "java-id",
			pod: &workloadmeta.KubernetesPod{
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "java-id",
						Name: "java-name",
					},
				},
				InitContainers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "java-id-2",
						Name: "java-name-2",
					},
				},
			},
			expectedName:    "java-name",
			isInitContainer: false,
			found:           true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, isInitContainer, ok := getContainerInfoFromPod(tt.containerID, tt.pod)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.isInitContainer, isInitContainer)
			assert.Equal(t, tt.found, ok)
		})
	}
}

func TestPodHasOwner(t *testing.T) {
	tests := []struct {
		name     string
		pod      *workloadmeta.KubernetesPod
		expected bool
	}{
		{
			name: "has one owner",
			pod: &workloadmeta.KubernetesPod{
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						ID:   "id",
						Name: "name",
						Kind: "kind",
					},
				},
			},
			expected: true,
		},
		{
			name: "has two owners",
			pod: &workloadmeta.KubernetesPod{
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						ID:   "id",
						Name: "name",
						Kind: "kind",
					},
					{
						ID:   "id-2",
						Name: "name-2",
						Kind: "kind",
					},
				},
			},
			expected: true,
		},
		{
			name:     "owner is nil",
			pod:      &workloadmeta.KubernetesPod{},
			expected: false,
		},
		{
			name:     "owner is empty",
			pod:      &workloadmeta.KubernetesPod{Owners: []workloadmeta.KubernetesPodOwner{}},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasOwner := podHasOwner(tt.pod)
			assert.Equal(t, tt.expected, hasOwner)
		})
	}
}
