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

	containerDetails := &containerDetails{
		containersLanguages: map[string]*languagesSet{
			"java": {
				languages: map[string]struct{}{
					"Java": {},
				},
			},
		},
	}

	podDetails := &podDetails{
		namespace:           "default",
		containersLanguages: containerDetails,
		ownerRef: &workloadmeta.KubernetesPodOwner{
			Name: "dummyrs",
			Kind: "replicaset",
			ID:   "dummyid",
		},
	}
	podName := "nginx"
	client.currentBatch.podDetails[podName] = podDetails
	client.flush()
	<-doneCh
	assert.Equal(t, []*pbgo.ParentLanguageAnnotationRequest{
		{
			PodDetails: []*pbgo.PodLanguageDetails{
				{
					Name:             podName,
					Namespace:        podDetails.namespace,
					ContainerDetails: podDetails.containersLanguages.toProto(),
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

	eventBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
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

	assert.NotEmpty(t, client.currentBatch.podDetails)
}

func TestGetContainerNameFromPod(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "java-id",
				Name: "java-name",
			},
		},
	}
	name, ok := getContainerNameFromPod("java-id", pod)

	assert.Equal(t, "java-name", name)
	assert.True(t, ok)
}

func TestPodHasOwner(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		Owners: []workloadmeta.KubernetesPodOwner{
			{
				ID:   "id",
				Name: "name",
				Kind: "kind",
			},
		},
	}
	hasOwner := podHasOwner(pod)

	assert.True(t, hasOwner)
}
