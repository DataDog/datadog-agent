// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Handler owns an accepted task through final result publication.
type Handler interface {
	HandleTask(ctx context.Context, task *types.Task)
}

// Server exposes the local executor IPC API.
type Server struct {
	executorpb.UnimplementedExecutorServer

	handler     Handler
	version     string
	idleTimeout time.Duration
	authToken   string
	onShutdown  func(string)

	active atomic.Int32

	stateMu      sync.Mutex
	shuttingDown bool

	mu        sync.Mutex
	idleTimer *time.Timer
	grpcSrv   *grpc.Server
	runCtx    context.Context
	wg        sync.WaitGroup
}

// NewServer creates a task executor IPC server.
func NewServer(handler Handler, version string, idleTimeout time.Duration, authToken string, onShutdown func(string)) *Server {
	return &Server{
		handler:     handler,
		version:     version,
		idleTimeout: idleTimeout,
		authToken:   authToken,
		onShutdown:  onShutdown,
	}
}

// Serve runs the executor gRPC API on a local listener.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	s.runCtx = ctx
	s.grpcSrv = grpc.NewServer()
	executorpb.RegisterExecutorServer(s.grpcSrv, s)
	s.resetIdleTimer()
	go func() {
		<-ctx.Done()
		_ = s.Stop(context.Background())
	}()

	err := s.grpcSrv.Serve(listener)
	if errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}

// Stop shuts down the executor IPC server.
func (s *Server) Stop(ctx context.Context) error {
	s.stopIdleTimer()
	if s.grpcSrv == nil {
		return nil
	}
	stopped := make(chan struct{})
	go func() {
		s.grpcSrv.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-ctx.Done():
		s.grpcSrv.Stop()
		return ctx.Err()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) Status(ctx context.Context, _ *executorpb.StatusRequest) (*executorpb.StatusResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	return statusResponse(s.active.Load(), s.version), nil
}

func (s *Server) SubmitTask(ctx context.Context, req *executorpb.SubmitTaskRequest) (*executorpb.SubmitTaskResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	taskJSON := req.GetTaskJson()
	task := &types.Task{Raw: taskJSON}
	if err := json.Unmarshal(taskJSON, task); err != nil {
		return &executorpb.SubmitTaskResponse{Accepted: false, Reason: "invalid task payload"}, nil
	}

	s.stateMu.Lock()
	if s.shuttingDown {
		s.stateMu.Unlock()
		return &executorpb.SubmitTaskResponse{Accepted: false, Reason: "executor is shutting down"}, nil
	}
	s.active.Add(1)
	s.wg.Add(1)
	s.stateMu.Unlock()

	s.stopIdleTimer()
	go func() {
		defer func() {
			if s.active.Add(-1) == 0 {
				s.resetIdleTimer()
			}
			s.wg.Done()
		}()
		s.handler.HandleTask(context.WithoutCancel(s.runCtx), task)
	}()

	return &executorpb.SubmitTaskResponse{Accepted: true}, nil
}

func (s *Server) Shutdown(ctx context.Context, _ *executorpb.StatusRequest) (*executorpb.StatusResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.stateMu.Lock()
	s.shuttingDown = true
	s.stateMu.Unlock()
	s.stopIdleTimer()
	s.wg.Wait()
	resp := statusResponse(s.active.Load(), s.version)
	go func() {
		if s.onShutdown != nil {
			s.onShutdown("executor shutdown requested, shutting down")
			return
		}
		_ = s.Stop(context.Background())
	}()
	return resp, nil
}

func (s *Server) authorize(ctx context.Context) error {
	if s.authToken == "" {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		for _, value := range md.Get("authorization") {
			if value == "Bearer "+s.authToken {
				return nil
			}
		}
	}
	return status.Error(codes.Unauthenticated, "unauthorized")
}

func (s *Server) resetIdleTimer() {
	if s.idleTimeout <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.idleTimer = time.AfterFunc(s.idleTimeout, func() {
		if s.active.Load() != 0 {
			return
		}
		if s.onShutdown != nil {
			s.onShutdown("executor idle timeout reached, shutting down")
		}
	})
}

func (s *Server) stopIdleTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
}

// LogShutdown logs and calls the supplied shutdown function.
func LogShutdown(ctx context.Context, shutdown func()) func(string) {
	return func(message string) {
		log.FromContext(ctx).Info(message)
		shutdown()
	}
}
