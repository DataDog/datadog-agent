// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
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

func newTestClient(t *testing.T, store workloadmeta.Store) (*client, *MockDCAClient, chan struct{}) {
	doneCh := make(chan struct{})
	mockDCAClient := &MockDCAClient{doneCh: doneCh}

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule,
		fx.Replace(config.MockParams{Overrides: map[string]interface{}{
			"language_detection.enabled":       "true",
			"cluster_agent.enabled":            "true",
			"language_detection.client_period": "50ms",
		}}),
		telemetry.MockModule,
		log.MockModule,
	))

	optComponent := newClient(deps).(util.Optional[Component])
	comp, _ := optComponent.Get()
	client := comp.(*client)
	client.langDetectionCl = mockDCAClient
	client.store = store

	return client, mockDCAClient, doneCh
}

func TestClientDisabled(t *testing.T) {
	testCases := []struct {
		languageEnabled     string
		clusterAgentEnabled string
		isSet               bool
	}{
		{"true", "true", true},
		{"true", "false", false},
		{"false", "true", false},
		{"false", "false", false},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("language_enabled=%s, cluster_agent_enabled=%s", testCase.languageEnabled, testCase.clusterAgentEnabled), func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				config.MockModule,
				fx.Replace(config.MockParams{Overrides: map[string]interface{}{
					"language_detection.enabled": testCase.languageEnabled,
					"cluster_agent.enabled":      testCase.clusterAgentEnabled,
				}}),
				telemetry.MockModule,
				log.MockModule,
			))

			optionalCl := newClient(deps).(util.Optional[Component])
			assert.Equal(t, testCase.isSet, optionalCl.IsSet())
		})
	}
}

func TestClientFlush(t *testing.T) {
	client, mockDCAClient, doneCh := newTestClient(t, nil)
	container := containerInfo{
		"java-cont": {
			"java": {},
		},
	}

	initContainer := containerInfo{
		"go-cont": {
			"go": {},
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
	client.currentBatch[podName] = podInfo

	// flush the batch as it is done in the client
	go client.startStreaming()
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
	// make sure we didn't touch the current batch
	client.mutex.Lock()
	defer client.mutex.Unlock()
	assert.Len(t, client.currentBatch, 0)
}

func TestClientProcessEvent(t *testing.T) {
	mockStore := workloadmeta.NewMockStore()
	client, _, _ := newTestClient(t, mockStore)

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
		ContainerID: "nginx-cont-id",
	}

	initProcess := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Language: &languagemodels.Language{
			Name: "go",
		},
		ContainerID: "init-nginx-cont-id",
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

	assert.NotEmpty(t, client.currentBatch)
	assert.Equal(t,
		batch{
			"nginx-pod-name": {
				namespace: "nginx-pod-namespace",
				containerInfo: containerInfo{
					"nginx-cont-name": {
						"java": {},
					},
				},
				initContainerInfo: containerInfo{
					"nginx-cont-name": {
						"go": {},
					},
				},
				ownerRef: &workloadmeta.KubernetesPodOwner{
					ID:   "nginx-replicaset-id",
					Name: "nginx-replicaset-name",
					Kind: "replicaset",
				},
			},
		},
		client.currentBatch,
	)
}

func TestGetContainerInfoFromPod(t *testing.T) {
	tests := []struct {
		name            string
		ContainerID     string
		pod             *workloadmeta.KubernetesPod
		expectedName    string
		isInitContainer bool
		found           bool
	}{
		{
			name:        "not found",
			ContainerID: "cid",
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
			ContainerID: "java-id-2",
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
			ContainerID: "java-id",
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
			name, isInitContainer, ok := getContainerInfoFromPod(tt.ContainerID, tt.pod)
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
