// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
)

type MockDCAClient struct {
	payload []*pbgo.ParentLanguageAnnotationRequest
	doneCh  chan struct{}
}

func (m *MockDCAClient) PostLanguageMetadata(_ context.Context, request *pbgo.ParentLanguageAnnotationRequest) error {
	m.payload = append(m.payload, request)
	go func() { m.doneCh <- struct{}{} }()
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

func TestClientEnabled(t *testing.T) {
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

func TestClientSend(t *testing.T) {
	client, mockDCAClient, doneCh := newTestClient(t, nil)
	container := langUtil.ContainersLanguages{
		"java-cont": {
			"java": {},
		},
	}

	initContainer := langUtil.ContainersLanguages{
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

	client.send(context.Background(), client.currentBatch.toProto())

	// wait that the mock dca client processes the message
	<-doneCh
	assert.Equal(t, []*pbgo.ParentLanguageAnnotationRequest{
		{
			PodDetails: []*pbgo.PodLanguageDetails{
				{
					Name:                 podName,
					Namespace:            podInfo.namespace,
					InitContainerDetails: podInfo.initContainerInfo.ToProto(),
					ContainerDetails:     podInfo.containerInfo.ToProto(),
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
	assert.Equal(t, client.currentBatch, batch{podName: podInfo})
}

func TestClientSendFreshPods(t *testing.T) {
	client, _, _ := newTestClient(t, nil)
	container := langUtil.ContainersLanguages{
		"java-cont": {
			"java": {},
		},
	}

	initContainer := langUtil.ContainersLanguages{
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

	// since `freshlyUpdatedPods` is empty, `getFreshBatchProto` should return Nil
	freshData := client.getFreshBatchProto()
	assert.Nil(t, freshData)

	client.freshlyUpdatedPods = map[string]struct{}{"nginx": {}}

	freshData = client.getFreshBatchProto()
	assert.Equal(t, &pbgo.ParentLanguageAnnotationRequest{
		PodDetails: []*pbgo.PodLanguageDetails{
			{
				Name:                 podName,
				Namespace:            podInfo.namespace,
				InitContainerDetails: podInfo.initContainerInfo.ToProto(),
				ContainerDetails:     podInfo.containerInfo.ToProto(),
				Ownerref: &pbgo.KubeOwnerInfo{
					Name: "dummyrs",
					Kind: "replicaset",
					Id:   "dummyid",
				},
			},
		},
	}, freshData)
	// make sure we didn't touch the current batch
	assert.Equal(t, client.currentBatch, batch{podName: podInfo})
	// make sure `freshlyUpdatedPods` is emptied
	assert.Empty(t, client.freshlyUpdatedPods)
}

func TestClientProcessEvent_EveryEntityStored(t *testing.T) {
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
				ID:   container.ID,
				Name: container.Name,
			},
		},
		InitContainers: []workloadmeta.OrchestratorContainer{
			{
				ID:   initContainer.ID,
				Name: initContainer.Name,
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
		ContainerID: container.ID,
	}

	initProcess := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "1234",
		},
		Language: &languagemodels.Language{
			Name: "go",
		},
		ContainerID: initContainer.ID,
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
			Entity: pod,
		},
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
	}

	mockStore.Notify(collectorEvents)

	client.processEvent(eventBundle)

	assert.NotEmpty(t, client.currentBatch)
	assert.Equal(t,
		batch{
			"nginx-pod-name": {
				namespace: "nginx-pod-namespace",
				containerInfo: langUtil.ContainersLanguages{
					"nginx-cont-name": {
						"java": {},
					},
				},
				initContainerInfo: langUtil.ContainersLanguages{
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
	assert.Empty(t, client.processesWithoutPod)
	assert.Equal(t, client.freshlyUpdatedPods, map[string]struct{}{"nginx-pod-name": {}})

	unsetPodEventBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Entity: pod,
				Type:   workloadmeta.EventTypeUnset,
			},
		},
		Ch: make(chan struct{}),
	}

	client.processEvent(unsetPodEventBundle)
	assert.Empty(t, client.currentBatch)
	assert.Empty(t, client.freshlyUpdatedPods)
}

func TestClientProcessEvent_PodMissing(t *testing.T) {
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
				ID:   container.ID,
				Name: container.Name,
			},
		},
		InitContainers: []workloadmeta.OrchestratorContainer{
			{
				ID:   initContainer.ID,
				Name: initContainer.Name,
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
			ID:   "012",
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
	}

	// store everything but the pod
	mockStore.Notify(collectorEvents)

	// process the events
	client.processEvent(eventBundle)

	// make sure the current batch is not updated
	assert.Empty(t, client.currentBatch)

	// make sure the events are added in `processesWithoutPod` so processing can be retried
	assert.Equal(t, eventBundle.Events, client.processesWithoutPod)
	assert.Empty(t, client.freshlyUpdatedPods)

	// add the pod in workloadmeta
	mockStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: pod,
		},
	})

	// retry processing processes without pod
	client.retryProcessEventsWithoutPod()
	assert.Equal(t,
		batch{
			"nginx-pod-name": {
				namespace: "nginx-pod-namespace",
				containerInfo: langUtil.ContainersLanguages{
					"nginx-cont-name": {
						"java": {},
					},
				},
				initContainerInfo: langUtil.ContainersLanguages{
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
	assert.Empty(t, client.processesWithoutPod)
	assert.Equal(t, client.freshlyUpdatedPods, map[string]struct{}{"nginx-pod-name": {}})

	unsetPodEventBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Entity: pod,
				Type:   workloadmeta.EventTypeUnset,
			},
		},
		Ch: make(chan struct{}),
	}

	client.processEvent(unsetPodEventBundle)
	assert.Empty(t, client.currentBatch)
	assert.Empty(t, client.freshlyUpdatedPods)
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

// TestRun checks that the client runs as expected and will help to identify potential data races
func TestRun(t *testing.T) {
	mockStore := workloadmeta.NewMockStore()
	client, mockDCAClient, doneCh := newTestClient(t, mockStore)
	client.newUpdatePeriod = 50 * time.Millisecond
	client.periodicalFlushPeriod = 1 * time.Second
	client.retryProcessWithoutPodPeriod = 100 * time.Millisecond

	err := client.start(context.Background())
	require.NoError(t, err)

	container1 := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "nginx-cont-id1",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "nginx-cont-name1",
		},
		Owner: &workloadmeta.EntityID{
			ID:   "nginx-pod-id1",
			Kind: workloadmeta.KindKubernetesPod,
		},
	}

	container2 := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "nginx-cont-id2",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "nginx-cont-name-2",
		},
		Owner: &workloadmeta.EntityID{
			ID:   "nginx-pod-id2",
			Kind: workloadmeta.KindKubernetesPod,
		},
	}

	pod1 := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			ID:   "nginx-pod-id1",
			Kind: workloadmeta.KindKubernetesPod,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "nginx-pod-name1",
			Namespace: "nginx-pod-namespace1",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "nginx-cont-id1",
				Name: "nginx-cont-name1",
			},
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{
				ID:   "nginx-replicaset-id1",
				Name: "nginx-replicaset-name1",
				Kind: "replicaset",
			},
		},
	}

	pod2 := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			ID:   "nginx-pod-id2",
			Kind: workloadmeta.KindKubernetesPod,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "nginx-pod-name2",
			Namespace: "nginx-pod-namespace2",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "nginx-cont-id2",
				Name: "nginx-cont-name2",
			},
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{
				ID:   "nginx-replicaset-id2",
				Name: "nginx-replicaset-name2",
				Kind: "replicaset",
			},
		},
	}

	process1 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "123",
		},
		Language: &languagemodels.Language{
			Name: "java",
		},
		ContainerID: "nginx-cont-id1",
	}

	processWithoutPod := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "1234",
		},
		Language: &languagemodels.Language{
			Name: "java",
		},
		ContainerID: "unknown-container",
	}

	process2 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "012",
		},
		Language: &languagemodels.Language{
			Name: "go",
		},
		ContainerID: "nginx-cont-id2",
	}

	collectorEvents1 := []workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: pod1,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: container1,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: process1,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: processWithoutPod,
		},
	}

	mockStore.Notify(collectorEvents1)

	// The entire batch should be sent with the first event, we can wait only once
	<-doneCh
	assert.Equal(t,
		batch{
			"nginx-pod-name1": {
				namespace: "nginx-pod-namespace1",
				containerInfo: langUtil.ContainersLanguages{
					"nginx-cont-name1": {
						"java": {},
					},
				},
				ownerRef: &workloadmeta.KubernetesPodOwner{
					ID:   "nginx-replicaset-id1",
					Name: "nginx-replicaset-name1",
					Kind: "replicaset",
				},
			},
		}.toProto(),
		mockDCAClient.payload[0],
	)

	collectorEvents2 := []workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: pod2,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: container2,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: process2,
		},
	}

	mockStore.Notify(collectorEvents2)

	b := batch{
		"nginx-pod-name2": {
			namespace: "nginx-pod-namespace2",
			containerInfo: langUtil.ContainersLanguages{
				"nginx-cont-name2": {
					"go": {},
				},
			},
			ownerRef: &workloadmeta.KubernetesPodOwner{
				ID:   "nginx-replicaset-id2",
				Name: "nginx-replicaset-name2",
				Kind: "replicaset",
			},
		},
		"nginx-pod-name1": {
			namespace: "nginx-pod-namespace1",
			containerInfo: langUtil.ContainersLanguages{
				"nginx-cont-name1": {
					"java": {},
				},
			},
			ownerRef: &workloadmeta.KubernetesPodOwner{
				ID:   "nginx-replicaset-id1",
				Name: "nginx-replicaset-name1",
				Kind: "replicaset",
			},
		},
	}

	// the periodic flush mechanism should send the entire data every 100ms
	assert.Eventually(t, func() bool {
		<-doneCh
		a := protoToBatch(mockDCAClient.payload[len(mockDCAClient.payload)-1])
		return a.equals(b)
	},
		5*time.Second,
		100*time.Millisecond,
	)

	unsetPodEvent := []workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceAll,
			Entity: pod2,
		},
	}

	mockStore.Notify(unsetPodEvent)

	// the periodic flush mechanism should send the up to date data after removing the pod
	b = batch{
		"nginx-pod-name1": {
			namespace: "nginx-pod-namespace1",
			containerInfo: langUtil.ContainersLanguages{
				"nginx-cont-name1": {
					"java": {},
				},
			},
			ownerRef: &workloadmeta.KubernetesPodOwner{
				ID:   "nginx-replicaset-id1",
				Name: "nginx-replicaset-name1",
				Kind: "replicaset",
			},
		},
	}

	assert.Eventually(t, func() bool {
		<-doneCh
		a := protoToBatch(mockDCAClient.payload[len(mockDCAClient.payload)-1])
		return a.equals(b)
	},
		5*time.Second,
		100*time.Millisecond,
	)

	client.stop(context.Background())
}

func protoToBatch(protoMessage *pbgo.ParentLanguageAnnotationRequest) batch {
	res := make(batch)

	for _, podDetail := range protoMessage.PodDetails {
		cInfo := langUtil.NewContainersLanguages()

		for _, container := range podDetail.ContainerDetails {
			languageSet := langUtil.NewLanguageSet()
			for _, lang := range container.Languages {
				languageSet.Add(lang.Name)
			}
			cInfo[container.ContainerName] = languageSet
		}

		initContainerInfo := langUtil.NewContainersLanguages()

		for _, container := range podDetail.InitContainerDetails {
			languageSet := langUtil.NewLanguageSet()
			for _, lang := range container.Languages {
				languageSet.Add(lang.Name)
			}
			initContainerInfo[container.ContainerName] = languageSet
		}

		podInfo := podInfo{
			namespace: podDetail.Namespace,
			ownerRef: &workloadmeta.KubernetesPodOwner{
				Kind: podDetail.Ownerref.Kind,
				ID:   podDetail.Ownerref.Id,
				Name: podDetail.Ownerref.Name,
			},
			containerInfo:     cInfo,
			initContainerInfo: initContainerInfo,
		}

		res[podDetail.Name] = &podInfo
	}

	return res
}

func (b batch) equals(other batch) bool {
	if len(b) != len(other) {
		return false
	}
	for podName, podInfo := range b {
		otherPodInfo, ok := other[podName]
		if !ok {
			return false
		}
		if !podInfo.equals(otherPodInfo) {
			return false
		}
	}
	return true
}

func (p *podInfo) equals(other *podInfo) bool {
	if p == other {
		return true
	}
	if p == nil || other == nil {
		return false
	}
	if p.namespace != other.namespace || !(*p.ownerRef == *other.ownerRef) || !p.containerInfo.Equals(other.containerInfo) || !p.initContainerInfo.Equals(other.initContainerInfo) {
		return false
	}
	return true
}
