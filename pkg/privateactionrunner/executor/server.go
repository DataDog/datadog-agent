// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package executor is the on-demand Go executor of the split Private Action Runner:
// a local gRPC server the Rust control plane dials to run one action.
package executor

import (
	"context"
	"encoding/json"
	"errors"
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

type actionExecutor interface {
	PrepareTask(ctx context.Context, task *types.Task) (*runners.PreparedWorkflowTask, *types.Task, error)
	RunPrepared(ctx context.Context, prepared *runners.PreparedWorkflowTask) (interface{}, error)
}

// Server implements the Executor gRPC service.
type Server struct {
	pb.UnimplementedExecutorServer

	executor actionExecutor
	version  string

	ready        atomic.Bool
	active       atomic.Int32
	lastActivity atomic.Int64 // unix-nanos of the last Health/RunAction
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

func (s *Server) touch() {
	s.lastActivity.Store(time.Now().UnixNano())
}

func (s *Server) idleFor() time.Duration {
	return time.Since(time.Unix(0, s.lastActivity.Load()))
}

// SetReady marks the executor ready (or not) to accept actions.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// Health reports readiness and liveness.
func (s *Server) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	s.touch()
	return &pb.HealthResponse{
		Ready:         s.ready.Load(),
		ActiveActions: s.active.Load(),
		Version:       s.version,
	}, nil
}

// RunAction verifies and runs a single action, streaming a terminal ActionResult back.
// Action failures come back as a structured error in the result, not a gRPC error.
func (s *Server) RunAction(req *pb.RunActionRequest, stream pb.Executor_RunActionServer) error {
	ctx := stream.Context()
	logger := log.FromContext(ctx)

	if !s.ready.Load() {
		return sendError(stream, util.NewPARError(
			aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR,
			errors.New("executor is not ready to accept actions"),
		))
	}

	s.touch()
	s.active.Add(1)
	defer func() {
		s.active.Add(-1)
		s.touch()
	}()

	// Raw bytes must stay unmodified for signature verification.
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

func sendError(stream pb.Executor_RunActionServer, parErr util.PARError) error {
	return stream.Send(&pb.RunActionResponse{
		Event: &pb.RunActionResponse_Result{
			Result: &pb.ActionResult{
				Outcome: &pb.ActionResult_Error{Error: parErr.ActionPlatformError},
			},
		},
	})
}

// ServeOptions tunes drain and idle-shutdown behavior; zero values disable each bound.
type ServeOptions struct {
	DrainTimeout        time.Duration // bounds graceful drain on stop; 0 waits forever
	IdleShutdownTimeout time.Duration // >0: self-exit after this long idle with no in-flight actions
}

// Serve serves the Executor on lis until ctx is cancelled or the orphan watchdog fires,
// then stops gracefully bounded by the drain timeout. Pass grpcOpts to secure the socket.
func Serve(ctx context.Context, lis net.Listener, srv *Server, opts ServeOptions, grpcOpts ...grpc.ServerOption) error {
	grpcServer := grpc.NewServer(grpcOpts...)
	pb.RegisterExecutorServer(grpcServer, srv)

	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if opts.IdleShutdownTimeout > 0 {
		go srv.watchIdle(serveCtx, cancel, opts.IdleShutdownTimeout)
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

// idlePollCap bounds how often watchIdle checks for inactivity.
const idlePollCap = 5 * time.Second

func (s *Server) watchIdle(ctx context.Context, cancel context.CancelFunc, idle time.Duration) {
	poll := idle / 2
	if poll > idlePollCap {
		poll = idlePollCap
	}
	if poll <= 0 {
		poll = time.Millisecond
	}
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
		grpcServer.Stop()
		<-done
	}
}
