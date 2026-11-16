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

	config "github.com/DataDog/datadog-agent/comp/core/config"
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

// fakeEventService is a containerd.EventService that records its
// subscriptions so tests can end each stream by hand.
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

// notifyingStore is a workloadmeta store that records notifications and
// serves a fixed set of containers.
type notifyingStore struct {
	workloadmeta.Component

	mu         sync.Mutex
	events     []workloadmeta.CollectorEvent
	containers []*workloadmeta.Container
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

func (s *notifyingStore) ListContainers() []*workloadmeta.Container {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*workloadmeta.Container{}, s.containers...)
}

func (s *notifyingStore) setContainers(containers ...*workloadmeta.Container) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.containers = containers
}

// streamTestHarness wires a collector to a fake containerd daemon whose
// event streams the test can end by hand.
type streamTestHarness struct {
	cfg          config.Component
	collector    *collector
	eventService *fakeEventService
	store        *notifyingStore
}

func newStreamTestHarness(t *testing.T) *streamTestHarness {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("container_image.enabled", false)
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)

	h := &streamTestHarness{
		cfg:          mockConfig,
		eventService: &fakeEventService{},
		store:        &notifyingStore{},
	}

	h.collector = &collector{
		id:                     collectorID,
		cfg:                    mockConfig,
		store:                  h.store,
		catalog:                workloadmeta.NodeAgent,
		contToExitInfo:         make(map[string]*exitInfo),
		knownImages:            newKnownImages(),
		imageSBOMs:             make(map[string]*workloadmeta.CompressedSBOM),
		filterPausedContainers: mockFilterStore.GetContainerPausedFilters(),
		filterSBOMContainers:   mockFilterStore.GetContainerSBOMFilters(),
		newClient:              h.newClient,
	}

	firstClient, err := h.newClient()
	require.NoError(t, err)
	h.collector.containerdClient = firstClient

	return h
}

func (h *streamTestHarness) newClient() (cutil.ContainerdItf, error) {
	return &fake.MockedContainerdClient{
		MockEvents: func() containerd.EventService {
			return h.eventService
		},
		MockNamespaces: func(context.Context) ([]string, error) {
			return []string{"default"}, nil
		},
		MockContainers: func(string) ([]containerd.Container, error) {
			return nil, nil
		},
		MockListImages: func(string) ([]containerd.Image, error) {
			return nil, nil
		},
		MockContainerWithCtx: func(_ context.Context, _, _ string) (containerd.Container, error) {
			return nil, agentErrors.NewNotFound("test")
		},
		MockClose: func() error {
			return nil
		},
	}, nil
}

// containerDeleteEnvelope builds an envelope for a container deletion event.
func containerDeleteEnvelope(t *testing.T, namespace string, containerID string) *containerdevents.Envelope {
	deletePayload, err := proto.Marshal(&apievents.ContainerDelete{ID: containerID})
	require.NoError(t, err)

	return &containerdevents.Envelope{
		Namespace: namespace,
		Topic:     containerDeletionTopic,
		Event: &types.Any{
			TypeUrl: "containerd.events.ContainerDelete",
			Value:   deletePayload,
		},
	}
}

// storeWasNotifiedWithUnset reports whether an Unset event was notified for
// an entity of the given kind and ID.
func storeWasNotifiedWithUnset(events []workloadmeta.CollectorEvent, kind workloadmeta.Kind, id string) bool {
	for _, event := range events {
		if event.Type != workloadmeta.EventTypeUnset {
			continue
		}
		if event.Entity.GetID().Kind == kind && event.Entity.GetID().ID == id {
			return true
		}
	}
	return false
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

// TestStreamRecoversFromStreamEOF is a regression test: when the containerd
// daemon restarts, the event stream ends with io.EOF and the collector must
// reconnect on a new client and keep consuming events instead of stopping
// forever.
func TestStreamRecoversFromStreamEOF(t *testing.T) {
	h := newStreamTestHarness(t)
	const namespace = "default"
	const deletedContainerID = "ben-bitdiddle"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, h.collector.subscribeAndResync(ctx))
	require.Equal(t, 1, h.eventService.subscribeCount())

	streamDone := make(chan struct{})
	go func() {
		h.collector.stream(ctx)
		close(streamDone)
	}()

	// The daemon restarts: the stream ends with io.EOF, and the collector
	// must reconnect on a new connection.
	firstSubscription := h.eventService.subscription(0)
	firstSubscription.errs <- io.EOF
	close(firstSubscription.errs)

	require.Eventually(t, func() bool {
		return h.eventService.subscribeCount() == 2
	}, 5*time.Second, 10*time.Millisecond)

	// Events from the new stream are still handled and notified.
	secondSubscription := h.eventService.subscription(1)
	secondSubscription.events <- containerDeleteEnvelope(t, namespace, deletedContainerID)

	require.Eventually(t, func() bool {
		return storeWasNotifiedWithUnset(h.store.notifiedEvents(), workloadmeta.KindContainer, deletedContainerID)
	}, 5*time.Second, 10*time.Millisecond)

	// The collector streams until its context is cancelled.
	select {
	case <-streamDone:
		t.Fatal("the collector stopped streaming after a stream EOF")
	default:
	}

	cancel()
	select {
	case <-streamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the collector did not stop after context cancellation")
	}
}

// TestResyncUnsetsEntitiesDeletedWhileStreamWasDown is a regression test:
// containerd does not replay the events emitted while no stream was open, so
// entities deleted during an outage must be unset from the store when it
// resynchronizes, or they stay there forever.
func TestResyncUnsetsEntitiesDeletedWhileStreamWasDown(t *testing.T) {
	h := newStreamTestHarness(t)
	h.cfg.SetInTest("container_image.enabled", true)

	const namespace = "default"
	const staleContainerID = "stale-container"
	const staleImageID = "sha256:stale-image"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, h.collector.subscribeAndResync(ctx))

	streamDone := make(chan struct{})
	go func() {
		h.collector.stream(ctx)
		close(streamDone)
	}()

	// Entities published before the outage, plus a container of another
	// runtime collector that must not be touched.
	h.store.setContainers(
		&workloadmeta.Container{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: staleContainerID},
			EntityMeta: workloadmeta.EntityMeta{Namespace: namespace},
			Runtime:    workloadmeta.ContainerRuntimeContainerd,
		},
		&workloadmeta.Container{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "docker-container"},
			EntityMeta: workloadmeta.EntityMeta{Namespace: namespace},
			Runtime:    workloadmeta.ContainerRuntimeDocker,
		},
	)
	h.collector.knownImages.addReference(staleImageID, staleImageID)
	h.collector.imageSBOMs[staleImageID] = &workloadmeta.CompressedSBOM{}

	// The daemon restarts with an empty snapshot, so both stale entities
	// no longer exist.
	firstSubscription := h.eventService.subscription(0)
	firstSubscription.errs <- io.EOF
	close(firstSubscription.errs)

	require.Eventually(t, func() bool {
		return h.eventService.subscribeCount() == 2
	}, 5*time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		notified := h.store.notifiedEvents()
		return storeWasNotifiedWithUnset(notified, workloadmeta.KindContainer, staleContainerID) &&
			storeWasNotifiedWithUnset(notified, workloadmeta.KindContainerImageMetadata, staleImageID)
	}, 5*time.Second, 10*time.Millisecond)

	assert.False(t, storeWasNotifiedWithUnset(h.store.notifiedEvents(), workloadmeta.KindContainer, "docker-container"))

	// The stale image is also gone from the collector's bookkeeping.
	_, found := h.collector.knownImages.getImageID(staleImageID)
	assert.False(t, found)
	_, hasSBOM := h.collector.imageSBOMs[staleImageID]
	assert.False(t, hasSBOM)

	cancel()
	select {
	case <-streamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the collector did not stop after context cancellation")
	}
}
