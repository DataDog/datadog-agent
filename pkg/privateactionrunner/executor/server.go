// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package executor implements the on-demand Go executor half of the split Private
// Action Runner. It exposes a local gRPC server (Executor service) that the
// always-on Rust control plane (par-control) dials to run a single action and
// stream the outcome back. The executor never talks to OPMS; it only verifies and
// runs actions via the shared execute-one-action core and reports structured
// results/errors over the RunAction stream.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"

	"google.golang.org/grpc"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

// ProtocolVersion is the control<->executor gRPC contract version reported by Health.
// Bump it on incompatible changes so the control plane can negotiate compatibility.
const ProtocolVersion uint32 = 1

// actionExecutor is the execute-one-action core the server dispatches to. It is
// satisfied by *runners.WorkflowTaskExecutor; tests inject a fake to exercise the
// gRPC streaming/error plumbing without running a real bundle.
type actionExecutor interface {
	PrepareTask(ctx context.Context, task *types.Task) (*runners.PreparedWorkflowTask, *types.Task, error)
	RunPrepared(ctx context.Context, prepared *runners.PreparedWorkflowTask) (interface{}, error)
}

// Server implements the Executor gRPC service.
type Server struct {
	pb.UnimplementedExecutorServer

	executor actionExecutor
	version  string

	ready  atomic.Bool
	active atomic.Int32
}

// NewServer builds a gRPC server that dispatches actions to the given core.
func NewServer(executor actionExecutor, version string) *Server {
	return &Server{
		executor: executor,
		version:  version,
	}
}

// SetReady marks the executor ready (or not) to accept actions. The lifecycle
// owner flips this to true once signing keys are loaded (RunAction is gated on it,
// and the control plane also gates dispatch on Health.ready).
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// Health reports readiness and liveness so the control plane can gate dispatch.
func (s *Server) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Ready:           s.ready.Load(),
		ProtocolVersion: ProtocolVersion,
		ActiveActions:   s.active.Load(),
		Version:         s.version,
	}, nil
}

// RunAction verifies and runs a single action, streaming a terminal ActionResult
// back. Verification, credential, allowlist, and timeout failures are returned as
// a structured ActionPlatformError in the result rather than a gRPC-level error, so
// the control plane can publish them to OPMS verbatim. A gRPC-level error is
// reserved for transport/protocol problems.
func (s *Server) RunAction(req *pb.RunActionRequest, stream pb.Executor_RunActionServer) error {
	ctx := stream.Context()
	logger := log.FromContext(ctx)

	if !s.ready.Load() {
		return sendError(stream, util.NewPARError(
			aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("executor is not ready to accept actions"),
		))
	}

	s.active.Add(1)
	defer s.active.Add(-1)

	// Parse the raw task bytes exactly as the OPMS dequeue path does; keep the raw
	// bytes on the task so signature verification sees the unmodified envelope.
	task := &types.Task{Raw: req.GetTask()}
	if err := json.Unmarshal(req.GetTask(), task); err != nil {
		logger.Error("could not parse task", log.ErrorField(err))
		return sendError(stream, util.NewPARError(
			aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("could not parse task: %w", err),
		))
	}

	prepared, _, err := s.executor.PrepareTask(ctx, task)
	if err != nil {
		return sendError(stream, util.DefaultPARError(err))
	}

	output, err := s.executor.RunPrepared(ctx, prepared)
	if err != nil {
		return sendError(stream, util.DefaultPARError(err))
	}

	outputBytes, err := json.Marshal(output)
	if err != nil {
		logger.Error("could not serialize action output", log.ErrorField(err))
		return sendError(stream, util.NewPARError(
			aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("could not serialize action output: %w", err),
		))
	}

	return stream.Send(&pb.RunActionResponse{
		Event: &pb.RunActionResponse_Result{
			Result: &pb.ActionResult{
				Outcome: &pb.ActionResult_Output{Output: outputBytes},
			},
		},
	})
}

// sendError streams a terminal ActionResult carrying the structured PAR error.
func sendError(stream pb.Executor_RunActionServer, parErr util.PARError) error {
	return stream.Send(&pb.RunActionResponse{
		Event: &pb.RunActionResponse_Result{
			Result: &pb.ActionResult{
				Outcome: &pb.ActionResult_Error{Error: parErr.ActionPlatformError},
			},
		},
	})
}

// Serve registers the Executor service on a fresh gRPC server and serves it on lis
// until ctx is cancelled, at which point it stops gracefully. mTLS is layered on in
// a later slice; slice 1 uses a plaintext local socket.
func Serve(ctx context.Context, lis net.Listener, srv *Server, opts ...grpc.ServerOption) error {
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterExecutorServer(grpcServer, srv)

	errCh := make(chan error, 1)
	go func() {
		errCh <- grpcServer.Serve(lis)
	}()

	select {
	case <-ctx.Done():
		grpcServer.GracefulStop()
		return nil
	case err := <-errCh:
		return err
	}
}
