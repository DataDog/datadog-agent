// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type blockingHandler struct {
	started chan struct{}
	release chan struct{}
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

func (h *blockingHandler) HandleTask(_ context.Context, _ *types.Task) {
	h.started <- struct{}{}
	<-h.release
}

func TestServerSubmitAcceptsTaskOwnership(t *testing.T) {
	handler := newBlockingHandler()
	server := NewServer(handler, "test-version", time.Minute, "", nil)
	server.runCtx = context.Background()

	resp := submitTask(t, server, context.Background(), sampleTaskJSON())
	require.True(t, resp.Accepted)

	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("task handler was not called")
	}
	close(handler.release)
}

func TestServerDoesNotEnforceCapacity(t *testing.T) {
	handler := newBlockingHandler()
	server := NewServer(handler, "test-version", time.Minute, "", nil)
	server.runCtx = context.Background()

	first := submitTask(t, server, context.Background(), sampleTaskJSON())
	require.True(t, first.Accepted)
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("first task handler was not called")
	}

	second := submitTask(t, server, context.Background(), sampleTaskJSON())
	require.True(t, second.Accepted)
	close(handler.release)
}

func TestServerIdleTimeoutCallsOnIdle(t *testing.T) {
	idle := make(chan struct{})
	server := NewServer(newBlockingHandler(), "test-version", 10*time.Millisecond, "", func(_ string) {
		close(idle)
	})
	server.resetIdleTimer()

	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("idle callback was not called")
	}
}

func TestServerShutdownCallsOnIdle(t *testing.T) {
	shutdown := make(chan struct{})
	server := NewServer(newBlockingHandler(), "test-version", time.Minute, "", func(_ string) {
		close(shutdown)
	})

	_, err := server.Shutdown(context.Background(), &executorpb.StatusRequest{})
	require.NoError(t, err)

	select {
	case <-shutdown:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not called")
	}
}

func TestServerShutdownWaitsForActiveTasks(t *testing.T) {
	handler := newBlockingHandler()
	shutdown := make(chan struct{})
	server := NewServer(handler, "test-version", time.Minute, "", func(_ string) {
		close(shutdown)
	})
	server.runCtx = context.Background()

	resp := submitTask(t, server, context.Background(), sampleTaskJSON())
	require.True(t, resp.Accepted)
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("task handler was not called")
	}

	done := make(chan struct{})
	go func() {
		_, err := server.Shutdown(context.Background(), &executorpb.StatusRequest{})
		require.NoError(t, err)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("shutdown completed before active task finished")
	case <-time.After(20 * time.Millisecond):
	}

	close(handler.release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not complete")
	}
	select {
	case <-shutdown:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not called")
	}
}

func TestServerRejectsInvalidAuthToken(t *testing.T) {
	server := NewServer(newBlockingHandler(), "test-version", time.Minute, "expected-token", nil)

	_, err := server.Status(context.Background(), &executorpb.StatusRequest{})

	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func submitTask(t *testing.T, server *Server, ctx context.Context, taskJSON []byte) *executorpb.SubmitTaskResponse {
	t.Helper()
	resp, err := server.SubmitTask(ctx, &executorpb.SubmitTaskRequest{TaskJson: taskJSON})
	require.NoError(t, err)
	return resp
}

func sampleTaskJSON() []byte {
	return []byte(`{"data":{"id":"task-1","attributes":{"name":"run","bundle_id":"com.datadoghq.test","job_id":"job-1"}}}`)
}
