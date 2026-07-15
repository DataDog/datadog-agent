// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package executor implements the on-demand Go executor half of the split Private
// Action Runner: a local gRPC server that the Rust control plane dials to run a
// single action and stream the outcome back.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

// ProtocolVersion is the control<->executor gRPC contract version reported by Health.
const ProtocolVersion uint32 = 1

// actionExecutor is the execute-one-action core the server dispatches to,
// satisfied by *runners.WorkflowTaskExecutor.
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
	// lastActivity is the unix-nanos timestamp of the last Health/RunAction.
	lastActivity atomic.Int64
}

// NewServer builds a gRPC server that dispatches actions to the given core.
func NewServer(executor actionExecutor, version string) *Server {
	s := &Server{
		executor: executor,
		version:  version,
	}
	s.touch()
	return s
}

// touch records activity for the orphan watchdog.
func (s *Server) touch() {
	s.lastActivity.Store(time.Now().UnixNano())
}

func (s *Server) idleFor() time.Duration {
	return time.Since(time.Unix(0, s.lastActivity.Load()))
}

// SetReady marks the executor ready (or not) to accept actions; RunAction is gated on it.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// Health reports readiness and liveness so the control plane can gate dispatch.
func (s *Server) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	s.touch()
	return &pb.HealthResponse{
		Ready:           s.ready.Load(),
		ProtocolVersion: ProtocolVersion,
		ActiveActions:   s.active.Load(),
		Version:         s.version,
	}, nil
}

// RunAction verifies and runs a single action, streaming a terminal ActionResult back.
// Action failures are returned as a structured ActionPlatformError in the result; a
// gRPC-level error is reserved for transport/protocol problems.
func (s *Server) RunAction(req *pb.RunActionRequest, stream pb.Executor_RunActionServer) error {
	ctx := stream.Context()
	logger := log.FromContext(ctx)

	if !s.ready.Load() {
		return sendError(stream, util.NewPARError(
			aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("executor is not ready to accept actions"),
		))
	}

	s.touch()
	s.active.Add(1)
	defer func() {
		s.active.Add(-1)
		s.touch()
	}()

	// Keep the raw bytes on the task so signature verification sees the unmodified envelope.
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

// ServeOptions tunes drain and orphan-safety behavior. Zero values disable the
// corresponding bound (wait forever to drain; never self-exit).
type ServeOptions struct {
	// DrainTimeout bounds graceful drain on stop; 0 waits indefinitely.
	DrainTimeout time.Duration
	// OrphanIdleTimeout, if > 0, self-exits after this long idle with no in-flight
	// actions, so a stray executor cannot linger if the control plane disappears.
	OrphanIdleTimeout time.Duration
	// PollInterval is how often the orphan watchdog checks (default 5s).
	PollInterval time.Duration
}

// Serve registers the Executor service on a fresh gRPC server and serves it on lis
// until ctx is cancelled or the orphan watchdog fires, then stops gracefully bounded
// by the drain timeout. Pass grpcOpts (e.g. TLS creds) to secure the socket.
func Serve(ctx context.Context, lis net.Listener, srv *Server, opts ServeOptions, grpcOpts ...grpc.ServerOption) error {
	grpcServer := grpc.NewServer(grpcOpts...)
	pb.RegisterExecutorServer(grpcServer, srv)

	// serveCtx is cancelled either by the caller (ctx) or by the orphan watchdog.
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if opts.OrphanIdleTimeout > 0 {
		poll := opts.PollInterval
		if poll <= 0 {
			poll = 5 * time.Second
		}
		go srv.watchOrphan(serveCtx, cancel, opts.OrphanIdleTimeout, poll)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- grpcServer.Serve(lis)
	}()

	select {
	case <-serveCtx.Done():
		stopGracefully(grpcServer, opts.DrainTimeout)
		return nil
	case err := <-errCh:
		return err
	}
}

// watchOrphan self-exits (cancels serveCtx) once the executor has been idle with no
// in-flight actions for longer than idle — the control plane is presumed gone.
func (s *Server) watchOrphan(ctx context.Context, cancel context.CancelFunc, idle, poll time.Duration) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.active.Load() == 0 && s.idleFor() >= idle {
				cancel()
				return
			}
		}
	}
}

// stopGracefully drains in-flight actions, force-stopping after drainTimeout.
func stopGracefully(grpcServer *grpc.Server, drainTimeout time.Duration) {
	done := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(done)
	}()
	if drainTimeout <= 0 {
		<-done
		return
	}
	select {
	case <-done:
	case <-time.After(drainTimeout):
		grpcServer.Stop() // force-stop wedged in-flight actions
		<-done
	}
}
