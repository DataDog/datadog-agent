// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockStream struct {
	ctx context.Context

	mu   sync.Mutex
	sent []*pb.WorkloadmetaStreamResponse

	// sendFunc overrides the default Send behavior
	sendFunc func(*pb.WorkloadmetaStreamResponse) error
}

func (m *mockStream) Send(resp *pb.WorkloadmetaStreamResponse) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendFunc != nil {
		return m.sendFunc(resp)
	}
	m.sent = append(m.sent, resp)
	return nil
}

func (m *mockStream) getSent() []*pb.WorkloadmetaStreamResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*pb.WorkloadmetaStreamResponse, len(m.sent))
	copy(result, m.sent)
	return result
}

func (m *mockStream) Context() context.Context     { return m.ctx }
func (m *mockStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockStream) SendHeader(metadata.MD) error { return nil }
func (m *mockStream) SetTrailer(metadata.MD)       {}
func (m *mockStream) SendMsg(any) error            { return nil }
func (m *mockStream) RecvMsg(any) error            { return nil }

func TestStreamEntities(t *testing.T) {
	t.Run("streams events to the client", func(t *testing.T) {
		store := newWorkloadmetaMock(t)
		server := newTestServer(store)
		testContainerID := "container-1"
		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()
		stream := &mockStream{ctx: ctx}

		done := make(chan error, 1)
		go func() {
			done <- server.StreamEntities(&pb.WorkloadmetaStreamRequest{}, stream)
		}()

		store.Set(&workloadmeta.Container{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: testContainerID},
		})

		assert.Eventually(t, func() bool {
			sent := stream.getSent()

			// The first Send is the initial snapshot from Subscribe (might be
			// empty). Look for at least one response containing the test
			// container ID.
			for _, resp := range sent {
				for _, ev := range resp.Events {
					if ev.GetContainer().GetEntityId().GetId() == testContainerID {
						return true
					}
				}
			}

			return false
		}, 5*time.Second, 50*time.Millisecond)

		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("StreamEntities did not return after context cancellation")
		}
	})

	t.Run("filters events by kind", func(t *testing.T) {
		store := newWorkloadmetaMock(t)
		s := newTestServer(store)
		testContainerID := "container-1"
		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()
		stream := &mockStream{ctx: ctx}

		// Subscribe only to containers
		req := &pb.WorkloadmetaStreamRequest{
			Filter: &pb.WorkloadmetaFilter{
				Kinds: []pb.WorkloadmetaKind{pb.WorkloadmetaKind_CONTAINER},
			},
		}

		done := make(chan error, 1)
		go func() {
			done <- s.StreamEntities(req, stream)
		}()

		// Store both a pod and a container
		store.Set(&workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-1"},
		})
		store.Set(&workloadmeta.Container{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: testContainerID},
		})

		assert.Eventually(t, func() bool {
			for _, resp := range stream.getSent() {
				for _, ev := range resp.Events {
					if ev.GetContainer().GetEntityId().GetId() == testContainerID {
						return true
					}
				}
			}
			return false
		}, 5*time.Second, 50*time.Millisecond)

		// Verify that the filter worked and the pod was not sent
		for _, resp := range stream.getSent() {
			for _, ev := range resp.Events {
				assert.Nil(t, ev.GetKubernetesPod())
			}
		}

		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("StreamEntities did not return after context cancellation")
		}
	})

	t.Run("returns error when send fails", func(t *testing.T) {
		store := newWorkloadmetaMock(t)
		s := newTestServer(store)
		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()

		stream := &mockStream{
			ctx:      ctx,
			sendFunc: func(*pb.WorkloadmetaStreamResponse) error { return errors.New("test error") },
		}

		done := make(chan error, 1)
		go func() {
			done <- s.StreamEntities(&pb.WorkloadmetaStreamRequest{}, stream)
		}()

		store.Set(&workloadmeta.Container{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container-1"},
		})

		select {
		case err := <-done:
			require.Error(t, err)
			assert.Contains(t, err.Error(), "test error")
		case <-time.After(5 * time.Second):
			t.Fatal("StreamEntities did not return after send error")
		}
	})

	t.Run("returns without error when context is cancelled", func(t *testing.T) {
		store := newWorkloadmetaMock(t)
		s := newTestServer(store)
		ctx, cancel := context.WithCancel(context.TODO())
		stream := &mockStream{ctx: ctx}

		done := make(chan error, 1)
		go func() {
			done <- s.StreamEntities(&pb.WorkloadmetaStreamRequest{}, stream)
		}()

		cancel()

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("StreamEntities did not return after context cancellation")
		}
	})

	t.Run("returns error when send queue is full", func(t *testing.T) {
		store := newWorkloadmetaMock(t)
		s := newTestServer(store)
		s.sendQueueSize = 1 // Size 1 to test easily the queue full case

		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()

		// Slow sender: takes longer than sendQueueTimeout to process each
		// event, to fill the queue.
		sendStarted := make(chan struct{}, 1)
		stream := &mockStream{
			ctx: ctx,
			sendFunc: func(*pb.WorkloadmetaStreamResponse) error {
				select {
				case sendStarted <- struct{}{}:
				default:
				}
				time.Sleep(2 * s.sendQueueTimeout)
				return nil
			},
		}

		done := make(chan error, 1)
		go func() {
			done <- s.StreamEntities(&pb.WorkloadmetaStreamRequest{}, stream)
		}()

		// Push the first event and wait for it to reach sendEvents (which
		// confirms StreamEntities has subscribed and events are flowing).
		store.Set(&workloadmeta.Container{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container-0"},
		})
		select {
		case <-sendStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("first event was not sent")
		}

		// Now sendEvents is blocked on the slow Send. Push 2 more events: the
		// first fills the queue, the second triggers the timeout.
		for _, container := range []string{"container-1", "container-2"} {
			store.Set(&workloadmeta.Container{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: container},
			})
		}

		select {
		case err := <-done:
			require.Error(t, err)
			assert.Contains(t, err.Error(), "send queue full")
		case <-time.After(5 * time.Second):
			t.Fatal("StreamEntities did not return after send queue timeout")
		}
	})
}

func newWorkloadmetaMock(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(context.TODO()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func newTestServer(store workloadmeta.Component) *Server {
	return &Server{
		wmeta:             store,
		streamSendTimeout: time.Second,
		sendQueueTimeout:  100 * time.Millisecond,
	}
}
