// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

type stopAwareExecutor struct {
	fakeExecutor
	entered, stopped chan struct{}
	once             sync.Once
}

func (e *stopAwareExecutor) Stop() { e.once.Do(func() { close(e.stopped) }) }
func (e *stopAwareExecutor) RunPrepared(ctx context.Context, _ *runners.PreparedWorkflowTask) (interface{}, error) {
	close(e.entered)
	select {
	case <-e.stopped:
		return nil, context.Canceled
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// RUN-04 executor-lifecycle wiring: notify the task executor before graceful
// draining, so remote queries can end their IPC calls. Existing drain behavior
// for other actions is covered by TestServeDrainsInFlightActionBeforeExit.
func TestServeNotifiesRunnerStopBeforeDrain(t *testing.T) {
	fake := &stopAwareExecutor{fakeExecutor: fakeExecutor{prepared: &runners.PreparedWorkflowTask{Task: &types.Task{}}}, entered: make(chan struct{}), stopped: make(chan struct{})}
	srv := NewServer(fake, "test-version")
	srv.SetReady(true)
	socketPath := testListenAddr(t)
	lis, err := Listen(socketPath)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, lis, srv, ServeOptions{DrainTimeout: time.Hour}) }()
	client := dialTestExecutor(t, socketPath)
	callCtx, cancelCall := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelCall()
	stream, err := client.RunAction(callCtx, &pb.RunActionRequest{Task: []byte(`{"data":{"id":"task"}}`)})
	require.NoError(t, err)
	select {
	case <-fake.entered:
	case <-callCtx.Done():
		t.Fatal("action did not start")
	}
	cancel()
	var result *pb.ActionResult
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if response.GetResult() != nil {
			result = response.GetResult()
		}
	}
	require.NotNil(t, result)
	require.NotNil(t, result.GetError())
	select {
	case err := <-served:
		require.NoError(t, err)
	case <-callCtx.Done():
		t.Fatal("executor waited for its drain timeout instead of stopping the action")
	}
}
