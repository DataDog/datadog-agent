// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
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
	server := NewServer(handler, "test-version", time.Minute, nil)
	server.runCtx = context.Background()

	resp := submitTask(t, server, sampleTaskJSON())
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
	server := NewServer(handler, "test-version", time.Minute, nil)
	server.runCtx = context.Background()

	first := submitTask(t, server, sampleTaskJSON())
	require.True(t, first.Accepted)
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("first task handler was not called")
	}

	second := submitTask(t, server, sampleTaskJSON())
	require.True(t, second.Accepted)
	close(handler.release)
}

func TestServerIdleTimeoutCallsOnIdle(t *testing.T) {
	idle := make(chan struct{})
	server := NewServer(newBlockingHandler(), "test-version", 10*time.Millisecond, func() {
		close(idle)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = server.Serve(ctx, listener)
	}()

	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("idle callback was not called")
	}
}

func submitTask(t *testing.T, server *Server, taskJSON []byte) *executorpb.SubmitTaskResponse {
	t.Helper()
	body, err := proto.Marshal(&executorpb.SubmitTaskRequest{TaskJson: taskJSON})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, submitPath, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleSubmit(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp executorpb.SubmitTaskResponse
	require.NoError(t, proto.Unmarshal(rec.Body.Bytes(), &resp))
	return &resp
}

func sampleTaskJSON() []byte {
	return []byte(`{"data":{"id":"task-1","attributes":{"name":"run","bundle_id":"com.datadoghq.test","job_id":"job-1"}}}`)
}
