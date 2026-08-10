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

	"github.com/benbjohnson/clock"
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

	ready  atomic.Bool
	active atomic.Int32

	// Unix nanoseconds of the last dispatch-related RPC, for the idle watchdog.
	lastActivity atomic.Int64
	clock        clock.Clock
}

// NewServer builds a gRPC server that dispatches actions to the given core.
func NewServer(executor actionExecutor, version string) *Server {
	s := &Server{
		executor: executor,
		version:  version,
		clock:    clock.New(),
	}
	// Start the idle clock at construction: an executor that is started and then
	// never dispatched to must still be reaped.
	s.touch()
	return s
}

// touch records dispatch-related activity for the idle watchdog.
//
// Health is deliberately excluded. The control plane probes it whenever it is
// deciding whether the executor is usable, so counting it would keep a
// permanently idle executor alive.
func (s *Server) touch() {
	s.lastActivity.Store(s.clock.Now().UnixNano())
}

// idleFor reports how long the executor has had no dispatch activity. An
// executor with an action in flight is never idle.
func (s *Server) idleFor() time.Duration {
	if s.active.Load() > 0 {
		return 0
	}
	return s.clock.Since(time.Unix(0, s.lastActivity.Load()))
}

// SetReady marks the executor ready (or not) to accept actions.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// Health reports readiness and liveness.
func (s *Server) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
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

// ServeOptions tunes drain behavior; zero value waits forever.
type ServeOptions struct {
	DrainTimeout time.Duration // bounds graceful drain on stop; 0 waits forever

	// OnIdleTimeout is called after the server has drained because IdleTimeout
	// elapsed. The run-executor command uses it to stop its enclosing lifecycle,
	// ensuring the whole process exits rather than only its gRPC server.
	OnIdleTimeout func()

	// IdleTimeout makes the executor exit by itself after this long with no
	// dispatch activity. 0 disables it, which is what the single-process
	// (non-split) runner wants.
	//
	// This is the backstop that guarantees an idle executor is always reaped.
	// The control plane normally stops it explicitly, but the two are siblings
	// under dd-procmgrd rather than parent and child, so nothing stops the
	// executor if the control plane is killed, hits its restart limit, or is
	// stopped on its own. The executor's process definition is `restart: never`,
	// so exiting once idle means it stays down until it is needed again.
	//
	// Set this longer than the control plane's own idle timeout so it only fires
	// when the control plane is not doing its job.
	IdleTimeout time.Duration
}

// idleCheckDivisor sets how often the idle watchdog samples, relative to the
// timeout, so that the executor exits reasonably promptly after going idle
// without polling tightly.
const idleCheckDivisor = 10

// Serve serves the Executor on lis until ctx is cancelled, then stops gracefully
// bounded by the drain timeout. Pass grpcOpts to secure the socket.
func Serve(ctx context.Context, lis net.Listener, srv *Server, opts ServeOptions, grpcOpts ...grpc.ServerOption) error {
	grpcServer := grpc.NewServer(grpcOpts...)
	pb.RegisterExecutorServer(grpcServer, srv)

	errCh := make(chan error, 1)
	go func() {
		errCh <- grpcServer.Serve(lis)
	}()

	idle := newIdleWatchdog(ctx, srv, opts.IdleTimeout)
	defer idle.stop()

	select {
	case <-ctx.Done():
		stopGracefully(grpcServer, opts.DrainTimeout)
		return nil
	case <-idle.fired:
		// GracefulStop lets an action that was accepted in the meantime finish.
		stopGracefully(grpcServer, opts.DrainTimeout)
		if opts.OnIdleTimeout != nil {
			opts.OnIdleTimeout()
		}
		return nil
	case err := <-errCh:
		return err
	}
}

type idleWatchdog struct {
	fired chan struct{}
	done  chan struct{}
}

// newIdleWatchdog closes `fired` once srv has been idle for timeout. A
// non-positive timeout yields a watchdog that never fires.
func newIdleWatchdog(ctx context.Context, srv *Server, timeout time.Duration) *idleWatchdog {
	w := &idleWatchdog{fired: make(chan struct{}), done: make(chan struct{})}
	if timeout <= 0 {
		return w
	}

	interval := timeout / idleCheckDivisor
	if interval <= 0 {
		interval = timeout
	}
	ticker := srv.clock.Ticker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.done:
				return
			case <-ticker.C:
				if srv.idleFor() >= timeout {
					close(w.fired)
					return
				}
			}
		}
	}()
	return w
}

func (w *idleWatchdog) stop() {
	close(w.done)
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
