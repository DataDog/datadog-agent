// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package clientimpl

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	clientComp "github.com/DataDog/datadog-agent/comp/languagedetection/client"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
)

type MockDCAClient struct {
	respCh chan *pbgo.ParentLanguageAnnotationRequest
}

func (m *MockDCAClient) PostLanguageMetadata(_ context.Context, request *pbgo.ParentLanguageAnnotationRequest) error {
	go func() { m.respCh <- request }()
	return nil
}

func newTestClient(t *testing.T) (*client, chan *pbgo.ParentLanguageAnnotationRequest) {
	respCh := make(chan *pbgo.ParentLanguageAnnotationRequest)
	mockDCAClient := &MockDCAClient{respCh: respCh}

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: map[string]interface{}{
			"language_detection.reporting.enabled":       "true",
			"language_detection.enabled":                 "true",
			"cluster_agent.enabled":                      "true",
			"language_detection.reporting.buffer_period": "50ms",
		}}),
		telemetry.MockModule(),
		logimpl.MockModule(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))

	optComponent := newClient(deps).(optional.Option[clientComp.Component])
	comp, _ := optComponent.Get()
	client := comp.(*client)
	client.langDetectionCl = mockDCAClient

	return client, respCh
}

func TestClientEnabled(t *testing.T) {
	testCases := []struct {
		languageDetectionEnabled          string
		languageDetectionReportingEnabled string
		clusterAgentEnabled               string
		isSet                             bool
	}{
		{"true", "true", "true", true},
		{"true", "true", "false", false},
		{"false", "true", "true", false},
		{"false", "true", "false", false},
		{"true", "false", "true", false},
		{"true", "false", "false", false},
		{"false", "false", "true", false},
		{"false", "false", "false", false},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf(
			"language_enabled=%s, language_detection_reporting_enabled=%s, cluster_agent_enabled=%s",
			testCase.languageDetectionEnabled,
			testCase.languageDetectionReportingEnabled,
			testCase.clusterAgentEnabled),
			func(t *testing.T) {
				deps := fxutil.Test[dependencies](t, fx.Options(
					config.MockModule(),
					fx.Replace(config.MockParams{Overrides: map[string]interface{}{
						"language_detection.enabled":           testCase.languageDetectionEnabled,
						"language_detection.reporting.enabled": testCase.languageDetectionReportingEnabled,
						"cluster_agent.enabled":                testCase.clusterAgentEnabled,
					}}),
					telemetry.MockModule(),
					logimpl.MockModule(),
					fx.Supply(workloadmeta.NewParams()),
					workloadmeta.MockModuleV2(),
				))

				optionalCl := newClient(deps).(optional.Option[clientComp.Component])
				_, ok := optionalCl.Get()
				assert.Equal(t, testCase.isSet, ok)
			})
	}
}

func TestClientSend(t *testing.T) {
	client, respCh := newTestClient(t)
	containers := langUtil.ContainersLanguages{
		langUtil.Container{
			Name: "java-cont",
			Init: false,
		}: {
			"java": {},
		},
		langUtil.Container{
			Name: "go-cont",
			Init: true,
		}: {
			"go": {},
		},
	}

	podInfo := &podInfo{
		namespace:     "default",
		containerInfo: containers,
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
	req := <-respCh

	containerDetails, initContainerDetails := podInfo.containerInfo.ToProto()
	assert.Equal(t, &pbgo.ParentLanguageAnnotationRequest{
		PodDetails: []*pbgo.PodLanguageDetails{
			{
				Name:                 podName,
				Namespace:            podInfo.namespace,
				ContainerDetails:     containerDetails,
				InitContainerDetails: initContainerDetails,
				Ownerref: &pbgo.KubeOwnerInfo{
					Name: "dummyrs",
					Kind: "replicaset",
					Id:   "dummyid",
				},
			},
		},
	}, req)
	// make sure we didn't touch the current batch
	assert.Equal(t, client.currentBatch, batch{podName: podInfo})
}

func TestClientSendFreshPods(t *testing.T) {
	client, _ := newTestClient(t)
	containers := langUtil.ContainersLanguages{
		langUtil.Container{
			Name: "java-cont",
			Init: false,
		}: {
			"java": {},
		},
		langUtil.Container{
			Name: "go-cont",
			Init: true,
		}: {
			"go": {},
		},
	}

	podInfo := &podInfo{
		namespace:     "default",
		containerInfo: containers,
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

	containerDetails, initContainerDetails := podInfo.containerInfo.ToProto()
	expectedFreshData := &pbgo.ParentLanguageAnnotationRequest{
		PodDetails: []*pbgo.PodLanguageDetails{
			{
				Name:                 podName,
				Namespace:            podInfo.namespace,
				ContainerDetails:     containerDetails,
				InitContainerDetails: initContainerDetails,
				Ownerref: &pbgo.KubeOwnerInfo{
					Name: "dummyrs",
					Kind: "replicaset",
					Id:   "dummyid",
				},
			},
		},
	}

	assert.Equal(t, expectedFreshData, freshData)
	// make sure we didn't touch the current batch
	assert.Equal(t, client.currentBatch, batch{podName: podInfo})
}

func TestClientProcessEvent_EveryEntityStored(t *testing.T) {
	client, _ := newTestClient(t)

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

	client.store.Notify(collectorEvents)

	client.handleEvent(eventBundle)

	assert.NotEmpty(t, client.currentBatch)
	assert.Equal(t,
		batch{
			"nginx-pod-name": {
				namespace: "nginx-pod-namespace",
				containerInfo: langUtil.ContainersLanguages{
					langUtil.Container{
						Name: "nginx-cont-name",
						Init: false,
					}: {
						"java": {},
					},

					langUtil.Container{
						Name: "nginx-cont-name",
						Init: true,
					}: {
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

	client.handleEvent(unsetPodEventBundle)
	assert.Empty(t, client.currentBatch)
	assert.Empty(t, client.freshlyUpdatedPods)
}

func TestClientProcessEvent_PodMissing(t *testing.T) {
	client, _ := newTestClient(t)

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
	client.store.Notify(collectorEvents)

	// process the events
	client.handleEvent(eventBundle)

	// make sure the current batch is not updated
	assert.Empty(t, client.currentBatch)

	// make sure the events are added in `processesWithoutPod` so processing can be retried
	assert.Len(t, client.processesWithoutPod, 2)
	assert.Empty(t, client.freshlyUpdatedPods)

	// add the pod in workloadmeta
	client.store.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: pod,
		},
	})

	// retry processing processes without pod
	client.retryProcessEventsWithoutPod([]string{"init-nginx-cont-id", "nginx-cont-id"})
	assert.Equal(t,
		batch{
			"nginx-pod-name": {
				namespace: "nginx-pod-namespace",
				containerInfo: langUtil.ContainersLanguages{
					langUtil.Container{
						Name: "nginx-cont-name",
						Init: false,
					}: {
						"java": {},
					},
					langUtil.Container{
						Name: "nginx-cont-name",
						Init: true,
					}: {
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

	client.handleEvent(unsetPodEventBundle)
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

func TestCleanUpProcesssesWithoutPod(t *testing.T) {
	ttl := 1 * time.Minute

	now := time.Now()
	past := now.Add(-ttl)
	future := now.Add(ttl)

	tests := []struct {
		name                string
		time                time.Time
		processesWithoutPod map[string]*eventsToRetry
		expected            map[string]*eventsToRetry
	}{
		{
			name: "has not expired",
			time: now,
			processesWithoutPod: map[string]*eventsToRetry{
				"a": {
					expirationTime: future,
				},
			},
			expected: map[string]*eventsToRetry{
				"a": {
					expirationTime: future,
				},
			},
		},
		{
			name: "has expired",
			time: now,
			processesWithoutPod: map[string]*eventsToRetry{
				"a": {
					expirationTime: past,
				},
			},
			expected: map[string]*eventsToRetry{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newTestClient(t)
			client.processesWithoutPod = tt.processesWithoutPod
			client.cleanUpProcesssesWithoutPod(tt.time)
			assert.Equal(t, tt.expected, client.processesWithoutPod)
		})
	}
}

// TestRun checks that the client runs as expected and will help to identify potential data races
func TestRun(t *testing.T) {
	client, respCh := newTestClient(t)
	client.freshDataPeriod = 50 * time.Millisecond
	client.periodicalFlushPeriod = 1 * time.Second
	client.processesWithoutPodCleanupPeriod = 100 * time.Millisecond

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

	container3 := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "python-cont-id3",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "python-cont-name3",
		},
		Owner: &workloadmeta.EntityID{
			ID:   "python-pod-id3",
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

	pod3 := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			ID:   "python-pod-id3",
			Kind: workloadmeta.KindKubernetesPod,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "python-pod-name3",
			Namespace: "python-pod-namespace3",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "python-cont-id3",
				Name: "python-cont-name3",
			},
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{
				ID:   "python-replicaset-id3",
				Name: "python-replicaset-name3",
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

	process3 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "1234",
		},
		Language: &languagemodels.Language{
			Name: "python",
		},
		ContainerID: "python-cont-id3",
	}

	collectorEvents1 := []workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: container1,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: pod1,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: process1,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: container1,
		},
		// Process 3 set event is here received before the set event of container 3 and pod 3
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: process3,
		},
	}

	client.store.Notify(collectorEvents1)

	expectedBatch := batch{
		"nginx-pod-name1": {
			namespace: "nginx-pod-namespace1",
			containerInfo: langUtil.ContainersLanguages{
				langUtil.Container{
					Name: "nginx-cont-name1",
					Init: false,
				}: {"java": {}},
			},
			ownerRef: &workloadmeta.KubernetesPodOwner{
				ID:   "nginx-replicaset-id1",
				Name: "nginx-replicaset-name1",
				Kind: "replicaset",
			},
		},
	}

	// The entire batch should be sent with the first event, we can wait only once
	req := <-respCh
	assert.True(t, expectedBatch.equals(protoToBatch(req)))

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
		// Now we receive the set events of container 3 and pod 3.
		// This should lead to retrying processing the process set event
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: container3,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceAll,
			Entity: pod3,
		},
	}

	client.store.Notify(collectorEvents2)

	b := batch{
		"nginx-pod-name2": {
			namespace: "nginx-pod-namespace2",
			containerInfo: langUtil.ContainersLanguages{
				langUtil.Container{
					Name: "nginx-cont-name2",
					Init: false,
				}: {"go": {}},
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
				langUtil.Container{
					Name: "nginx-cont-name1",
					Init: false,
				}: {"java": {}},
			},
			ownerRef: &workloadmeta.KubernetesPodOwner{
				ID:   "nginx-replicaset-id1",
				Name: "nginx-replicaset-name1",
				Kind: "replicaset",
			},
		},
		"python-pod-name3": {
			namespace: "python-pod-namespace3",
			containerInfo: langUtil.ContainersLanguages{
				langUtil.Container{
					Name: "python-cont-name3",
					Init: false,
				}: {"python": {}},
			},
			ownerRef: &workloadmeta.KubernetesPodOwner{
				ID:   "python-replicaset-id3",
				Name: "python-replicaset-name3",
				Kind: "replicaset",
			},
		},
	}

	// the periodic flush mechanism should send the entire data every 100ms
	assert.Eventually(t, func() bool {
		req := <-respCh
		a := protoToBatch(req)
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
		{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceAll,
			Entity: pod3,
		},
	}

	client.store.Notify(unsetPodEvent)

	// the periodic flush mechanism should send the up to date data after removing the pod
	b = batch{
		"nginx-pod-name1": {
			namespace: "nginx-pod-namespace1",
			containerInfo: langUtil.ContainersLanguages{
				langUtil.Container{
					Name: "nginx-cont-name1",
					Init: false,
				}: {"java": {}},
			},
			ownerRef: &workloadmeta.KubernetesPodOwner{
				ID:   "nginx-replicaset-id1",
				Name: "nginx-replicaset-name1",
				Kind: "replicaset",
			},
		},
	}

	assert.Eventually(t, func() bool {
		req := <-respCh
		a := protoToBatch(req)
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
		cInfo := make(langUtil.ContainersLanguages)

		for _, container := range podDetail.ContainerDetails {
			languageSet := make(langUtil.LanguageSet)
			for _, lang := range container.Languages {
				languageSet.Add(langUtil.Language(lang.Name))
			}
			cInfo[*langUtil.NewContainer(container.ContainerName)] = languageSet
		}

		for _, container := range podDetail.InitContainerDetails {
			languageSet := make(langUtil.LanguageSet)
			for _, lang := range container.Languages {
				languageSet.Add(langUtil.Language(lang.Name))
			}
			cInfo[*langUtil.NewContainer(container.ContainerName)] = languageSet
		}

		podInfo := podInfo{
			namespace: podDetail.Namespace,
			ownerRef: &workloadmeta.KubernetesPodOwner{
				Kind: podDetail.Ownerref.Kind,
				ID:   podDetail.Ownerref.Id,
				Name: podDetail.Ownerref.Name,
			},
			containerInfo: cInfo,
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
	if p.namespace != other.namespace || !(*p.ownerRef == *other.ownerRef) || !reflect.DeepEqual(p.containerInfo, other.containerInfo) {
		return false
	}
	return true
}
