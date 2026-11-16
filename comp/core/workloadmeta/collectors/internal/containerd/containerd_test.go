// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	apievents "github.com/containerd/containerd/api/events"
	containerd "github.com/containerd/containerd/v2/client"
	containerdcontainers "github.com/containerd/containerd/v2/core/containers"
	containerdevents "github.com/containerd/containerd/v2/core/events"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	agentErrors "github.com/DataDog/datadog-agent/pkg/errors"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
)

// fakeSubscription is one stream handed out by fakeEventService.
type fakeSubscription struct {
	events chan *containerdevents.Envelope
	errs   chan error
}

// fakeEventService is a containerd.EventService whose subscriptions are
// recorded so tests can decide when each stream ends and what it delivers.
type fakeEventService struct {
	containerdevents.Publisher
	containerdevents.Forwarder

	mu            sync.Mutex
	subscriptions []*fakeSubscription
}

func (s *fakeEventService) Subscribe(context.Context, ...string) (<-chan *containerdevents.Envelope, <-chan error) {
	sub := &fakeSubscription{
		events: make(chan *containerdevents.Envelope),
		errs:   make(chan error, 1),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions = append(s.subscriptions, sub)
	return sub.events, sub.errs
}

func (s *fakeEventService) subscription(index int) *fakeSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subscriptions[index]
}

func (s *fakeEventService) subscribeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subscriptions)
}

// notifyingStore is a workloadmeta store that records the events it is
// notified with.
type notifyingStore struct {
	workloadmeta.Component

	mu     sync.Mutex
	events []workloadmeta.CollectorEvent
}

func (s *notifyingStore) Notify(events []workloadmeta.CollectorEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
}

func (s *notifyingStore) notifiedEvents() []workloadmeta.CollectorEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]workloadmeta.CollectorEvent{}, s.events...)
}

func TestIgnoreContainer(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("container_exclude", []string{"name:agent-excluded"})
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)

	containerID := "123"

	container := mockedContainer{
		mockID: func() string {
			return containerID
		},
	}

	tests := []struct {
		name           string
		imgName        string
		isSandbox      bool
		container      containerd.Container
		expectsIgnored bool
	}{
		{
			name:           "pause image",
			imgName:        "k8s.gcr.io/pause",
			container:      &container,
			isSandbox:      false,
			expectsIgnored: true,
		},
		{
			name:           "is sandbox",
			imgName:        "k8s.gcr.io/pause",
			container:      &container,
			isSandbox:      true,
			expectsIgnored: true,
		},
		{
			name:           "non-pause container that exists",
			imgName:        "datadog/agent",
			container:      &container,
			isSandbox:      false,
			expectsIgnored: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := fake.MockedContainerdClient{
				MockInfo: func(string, containerd.Container) (containerdcontainers.Container, error) {
					return containerdcontainers.Container{
						Image: test.imgName,
					}, nil
				},
				MockIsSandbox: func(string, containerd.Container) (bool, error) {
					return test.isSandbox, nil
				},
			}

			containerdCollector := collector{
				containerdClient:       &client,
				filterPausedContainers: mockFilterStore.GetContainerPausedFilters(),
			}

			ignored, err := containerdCollector.ignoreContainer("default", test.container)
			assert.NoError(t, err)

			if test.expectsIgnored {
				assert.True(t, ignored)
			} else {
				assert.False(t, ignored)
			}
		})
	}
}

// TestStreamRecoversFromStreamEOF is a regression test that verifies that
// the collector reconnects when the containerd event stream ends with io.EOF.
func TestStreamRecoversFromStreamEOF(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("container_image.enabled", false)

	mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)

	const namespace = "default"
	const deletedContainerID = "ben-bitdiddle"

	eventService := &fakeEventService{}
	store := &notifyingStore{}

	// newClient mimics connecting to the containerd socket: every call
	// returns a fresh client over the same fake daemon, and the test records
	// which clients were created and closed.
	var (
		mu             sync.Mutex
		createdClients []*fake.MockedContainerdClient
		closedClients  []*fake.MockedContainerdClient
	)
	newClient := func() (cutil.ContainerdItf, error) {
		var client *fake.MockedContainerdClient
		client = &fake.MockedContainerdClient{
			MockEvents: func() containerd.EventService {
				return eventService
			},
			MockNamespaces: func(context.Context) ([]string, error) {
				return []string{namespace}, nil
			},
			MockContainers: func(string) ([]containerd.Container, error) {
				return nil, nil
			},
			MockContainerWithCtx: func(_ context.Context, _, _ string) (containerd.Container, error) {
				return nil, agentErrors.NewNotFound(deletedContainerID)
			},
			MockClose: func() error {
				mu.Lock()
				defer mu.Unlock()
				closedClients = append(closedClients, client)
				return nil
			},
		}

		mu.Lock()
		createdClients = append(createdClients, client)
		mu.Unlock()
		return client, nil
	}

	firstClient, err := newClient()
	require.NoError(t, err)

	c := &collector{
		id:                     collectorID,
		cfg:                    mockConfig,
		store:                  store,
		catalog:                workloadmeta.NodeAgent,
		containerdClient:       firstClient,
		contToExitInfo:         make(map[string]*exitInfo),
		knownImages:            newKnownImages(),
		imageSBOMs:             make(map[string]*workloadmeta.CompressedSBOM),
		filterPausedContainers: mockFilterStore.GetContainerPausedFilters(),
		filterSBOMContainers:   mockFilterStore.GetContainerSBOMFilters(),
		newClient:              newClient,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, c.subscribeAndResync(ctx))
	require.Equal(t, 1, eventService.subscribeCount())

	streamDone := make(chan struct{})
	go func() {
		c.stream(ctx)
		close(streamDone)
	}()

	// The containerd daemon restarts
	firstSubscription := eventService.subscription(0)
	firstSubscription.errs <- io.EOF
	close(firstSubscription.errs)

	// The collector must reconnect on a new client and resync the store
	require.Eventually(t, func() bool {
		return eventService.subscribeCount() == 2
	}, 5*time.Second, 10*time.Millisecond)

	mu.Lock()
	assert.Len(t, createdClients, 2)
	assert.Len(t, closedClients, 1)
	assert.Same(t, createdClients[0], closedClients[0])
	mu.Unlock()

	// Events from the new stream must still be handled and notified
	deletePayload, err := proto.Marshal(&apievents.ContainerDelete{ID: deletedContainerID})
	require.NoError(t, err)

	secondSubscription := eventService.subscription(1)
	secondSubscription.events <- &containerdevents.Envelope{
		Namespace: namespace,
		Topic:     containerDeletionTopic,
		Event: &types.Any{
			TypeUrl: "containerd.events.ContainerDelete",
			Value:   deletePayload,
		},
	}

	require.Eventually(t, func() bool {
		for _, event := range store.notifiedEvents() {
			if event.Type != workloadmeta.EventTypeUnset {
				continue
			}
			container, ok := event.Entity.(*workloadmeta.Container)
			if ok && container.ID == deletedContainerID {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)

	// The collector must not have stopped streaming after the EOF.
	select {
	case <-streamDone:
		t.Fatal("the collector stopped streaming after a stream EOF")
	default:
	}

	// The collector stops when its context is cancelled, not before.
	cancel()
	select {
	case <-streamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the collector did not stop after context cancellation")
	}
}
