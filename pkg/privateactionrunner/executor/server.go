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
	"net/http"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	executorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	"go.uber.org/atomic"
)

// Handler owns an accepted task through final result publication.
type Handler interface {
	HandleTask(ctx context.Context, task *types.Task)
}

// Server exposes the local executor IPC API.
type Server struct {
	handler     Handler
	version     string
	idleTimeout time.Duration
	onIdle      func()

	active atomic.Int32

	mu        sync.Mutex
	idleTimer *time.Timer
	httpSrv   *http.Server
	runCtx    context.Context
	wg        sync.WaitGroup
}

// NewServer creates a task executor IPC server.
func NewServer(handler Handler, version string, idleTimeout time.Duration, onIdle func()) *Server {
	return &Server{
		handler:     handler,
		version:     version,
		idleTimeout: idleTimeout,
		onIdle:      onIdle,
	}
}

// Serve runs the executor HTTP API on a local listener.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	s.runCtx = ctx
	mux := http.NewServeMux()
	mux.HandleFunc(statusPath, s.handleStatus)
	mux.HandleFunc(submitPath, s.handleSubmit)

	s.httpSrv = &http.Server{
		Handler: mux,
	}
	s.resetIdleTimer()
	go func() {
		<-ctx.Done()
		_ = s.Stop(context.Background())
	}()

	err := s.httpSrv.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Stop shuts down the executor IPC server.
func (s *Server) Stop(ctx context.Context) error {
	s.stopIdleTimer()
	if s.httpSrv == nil {
		return nil
	}
	err := s.httpSrv.Shutdown(ctx)
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return err
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeProto(w, statusResponse(s.active.Load(), s.version))
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req executorpb.SubmitTaskRequest
	if err := readProto(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task := &types.Task{Raw: req.TaskJson}
	if err := json.Unmarshal(req.TaskJson, task); err != nil {
		writeProto(w, &executorpb.SubmitTaskResponse{Accepted: false, Reason: "invalid task payload"})
		return
	}

	s.stopIdleTimer()
	s.active.Add(1)
	s.wg.Add(1)
	go func() {
		defer func() {
			if s.active.Add(-1) == 0 {
				s.resetIdleTimer()
			}
			s.wg.Done()
		}()
		s.handler.HandleTask(context.WithoutCancel(s.runCtx), task)
	}()

	writeProto(w, &executorpb.SubmitTaskResponse{Accepted: true})
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
		if s.onIdle != nil {
			s.onIdle()
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

// LogIdleShutdown logs and calls the supplied shutdown function.
func LogIdleShutdown(ctx context.Context, shutdown func()) func() {
	return func() {
		log.FromContext(ctx).Info("executor idle timeout reached, shutting down")
		shutdown()
	}
}
