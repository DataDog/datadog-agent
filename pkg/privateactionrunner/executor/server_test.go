// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package executor

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

// stubHandler is a TaskHandler test double that records calls and lets
// individual tests control the Execute behavior.
type stubHandler struct {
	prepared  atomic.Bool
	execCalls atomic.Int32
	executeFn func(ctx context.Context, task *types.Task) (interface{}, error)
}

func (h *stubHandler) Prepare(_ context.Context) error {
	h.prepared.Store(true)
	return nil
}

func (h *stubHandler) Execute(ctx context.Context, task *types.Task) (interface{}, error) {
	h.execCalls.Add(1)
	if h.executeFn != nil {
		return h.executeFn(ctx, task)
	}
	return nil, nil
}

// startTestServer spins up a Server, returns a client wired to it, and
// registers cleanup. authToken is left empty so auth is bypassed.
func startTestServer(t *testing.T, fn func(ctx context.Context, task *types.Task) (interface{}, error)) (executorpb.ExecutorClient, *stubHandler) {
	t.Helper()
	h := &stubHandler{executeFn: fn}
	// Use an actual Server but with the stub via a thin adapter: we
	// build a Server that wraps a *TaskHandler. Since TaskHandler is a
	// concrete struct, we cannot inject the stub directly — instead we
	// implement the Server's Execute logic inline using the stub by
	// wrapping it in a Server whose handler is nil and overriding via
	// a custom test adapter. The simplest path is to construct a
	// Server with a real TaskHandler-shaped value and let the stub
	// drive it. For these tests we instead build a thin gRPC server
	// directly against the stub: it lets us test the proto contract
	// (Execute success / error / cancel) without the full handler
	// surface.
	srv := &testServer{handler: h}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	gs := grpc.NewServer()
	executorpb.RegisterExecutorServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	t.Cleanup(func() {
		gs.GracefulStop()
		_ = conn.Close()
	})
	return executorpb.NewExecutorClient(conn), h
}

// testServer mirrors Server.Execute against a stubHandler so the tests
// exercise the proto surface without building a real TaskHandler.
type testServer struct {
	executorpb.UnimplementedExecutorServer
	handler *stubHandler
}

func (s *testServer) Execute(ctx context.Context, req *executorpb.ExecuteRequest) (*executorpb.ExecuteResponse, error) {
	task := &types.Task{}
	_ = json.Unmarshal(req.GetTaskJson(), task)
	task.Raw = req.GetTaskJson()
	output, err := s.handler.Execute(ctx, task)
	if err != nil {
		parErr := util.DefaultPARError(err)
		return &executorpb.ExecuteResponse{
			Error: &executorpb.ExecuteError{
				Code:            int32(parErr.ErrorCode),
				Message:         parErr.Message,
				ExternalMessage: parErr.ExternalMessage,
			},
		}, nil
	}
	outJSON, _ := json.Marshal(output)
	return &executorpb.ExecuteResponse{OutputJson: outJSON}, nil
}

func taskJSON(t *testing.T, id string) []byte {
	t.Helper()
	tk := &types.Task{}
	tk.Data.ID = id
	raw, err := json.Marshal(tk)
	require.NoError(t, err)
	return raw
}

// TestExecuteRPC_SuccessRoundtrip verifies the Execute RPC carries the
// task to the handler and returns the marshalled output.
func TestExecuteRPC_SuccessRoundtrip(t *testing.T) {
	client, h := startTestServer(t, func(_ context.Context, _ *types.Task) (interface{}, error) {
		return map[string]string{"hello": "world"}, nil
	})

	resp, err := client.Execute(context.Background(), &executorpb.ExecuteRequest{TaskJson: taskJSON(t, "task-1")})
	require.NoError(t, err)
	assert.Nil(t, resp.GetError())
	var out map[string]string
	require.NoError(t, json.Unmarshal(resp.GetOutputJson(), &out))
	assert.Equal(t, "world", out["hello"])
	assert.EqualValues(t, 1, h.execCalls.Load())
}

// TestExecuteRPC_ErrorIsConveyed verifies PARError fields survive the
// roundtrip into ExecuteError.
func TestExecuteRPC_ErrorIsConveyed(t *testing.T) {
	client, _ := startTestServer(t, func(_ context.Context, _ *types.Task) (interface{}, error) {
		return nil, util.NewPARError(aperrorpb.ActionPlatformErrorCode_ACTION_ERROR, errors.New("kaboom"))
	})

	resp, err := client.Execute(context.Background(), &executorpb.ExecuteRequest{TaskJson: taskJSON(t, "task-err")})
	require.NoError(t, err)
	require.NotNil(t, resp.GetError())
	assert.EqualValues(t, aperrorpb.ActionPlatformErrorCode_ACTION_ERROR, resp.GetError().GetCode())
	assert.Equal(t, "kaboom", resp.GetError().GetMessage())
	assert.Empty(t, resp.GetOutputJson())
}

// TestExecuteRPC_CtxCancelPropagatesToHandler verifies that cancelling
// the client-side ctx propagates into the handler's ctx, so the action
// can observe the cancel.
func TestExecuteRPC_CtxCancelPropagatesToHandler(t *testing.T) {
	released := make(chan struct{})
	observed := make(chan struct{})
	client, _ := startTestServer(t, func(ctx context.Context, _ *types.Task) (interface{}, error) {
		select {
		case <-ctx.Done():
			close(observed)
			return nil, ctx.Err()
		case <-released:
			return nil, nil
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.Execute(ctx, &executorpb.ExecuteRequest{TaskJson: taskJSON(t, "task-cancel")})
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatalf("handler did not observe cancel")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Execute did not return after cancel")
	}
	close(released)
}

// TestNewExecutor_UnknownModeRejected verifies invalid modes are
// rejected at construction time.
func TestNewExecutor_UnknownModeRejected(t *testing.T) {
	_, err := NewExecutor(Params{Mode: "stargate"})
	assert.Error(t, err)
}

// TestNewExecutor_InProcessRequiresHandler verifies in-process mode
// rejects a nil handler.
func TestNewExecutor_InProcessRequiresHandler(t *testing.T) {
	_, err := NewExecutor(Params{Mode: ModeInProcess})
	assert.Error(t, err)
}
