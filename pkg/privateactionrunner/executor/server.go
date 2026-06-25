// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"google.golang.org/grpc"
)

// Server is the gRPC server that runs inside the executor child process.
// It exposes a single Execute RPC: each incoming request runs the task
// through a TaskHandler and returns the result. There is no active-task
// bookkeeping or shutdown dance — the orchestrator owns drain coordination
// (it waits for its own in-flight Execute calls to return before stopping
// the supervisor) and gRPC's GracefulStop waits for in-flight handlers.
type Server struct {
	executorpb.UnimplementedExecutorServer

	handler   *TaskHandler
	authToken string

	mu      sync.Mutex
	grpc    *grpc.Server
	ready   bool
	stopped bool
}

// NewServer builds an Executor gRPC server backed by the given handler.
// An empty authToken disables auth (useful in tests).
func NewServer(handler *TaskHandler, authToken string) *Server {
	return &Server{handler: handler, authToken: authToken}
}

// Serve prepares the handler and starts the gRPC server on the given
// listener. It blocks until the listener is closed or Stop is called.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if err := s.handler.Prepare(ctx); err != nil {
		return fmt.Errorf("prepare executor handler: %w", err)
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return errors.New("executor server stopped before Serve")
	}
	s.ready = true
	s.grpc = grpc.NewServer()
	executorpb.RegisterExecutorServer(s.grpc, s)
	gs := s.grpc
	s.mu.Unlock()

	log.FromContext(ctx).Info("Executor IPC server listening", log.String("addr", listener.Addr().String()))
	if err := gs.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return nil
}

// Stop gracefully stops the gRPC server, which itself waits for in-flight
// Execute handlers to finish before returning.
func (s *Server) Stop(_ context.Context) error {
	s.mu.Lock()
	s.stopped = true
	gs := s.grpc
	s.mu.Unlock()
	if gs != nil {
		gs.GracefulStop()
	}
	return nil
}

// Execute implements executor.Executor/Execute. It runs the dequeued task
// to completion and returns the action output (or an error shape the
// orchestrator can publish).
func (s *Server) Execute(ctx context.Context, req *executorpb.ExecuteRequest) (*executorpb.ExecuteResponse, error) {
	if err := checkAuth(ctx, s.authToken); err != nil {
		return nil, err
	}

	task := &types.Task{}
	if err := json.Unmarshal(req.GetTaskJson(), task); err != nil {
		return executeError(util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("decode task: %w", err))), nil
	}
	task.Raw = req.GetTaskJson()

	output, err := s.handler.Execute(ctx, task)
	if err != nil {
		return executeError(err), nil
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		return executeError(util.NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, fmt.Errorf("marshal action output: %w", err))), nil
	}
	return &executorpb.ExecuteResponse{OutputJson: outputJSON}, nil
}

// executeError wraps any error into the proto ExecuteError shape the
// orchestrator expects.
func executeError(e error) *executorpb.ExecuteResponse {
	parErr := util.DefaultPARError(e)
	return &executorpb.ExecuteResponse{
		Error: &executorpb.ExecuteError{
			Code:            int32(parErr.ErrorCode),
			Message:         parErr.Message,
			ExternalMessage: parErr.ExternalMessage,
		},
	}
}
