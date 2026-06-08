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
	authToken   string
	onShutdown  func(string)

	active atomic.Int32

	stateMu      sync.Mutex
	shuttingDown bool

	mu        sync.Mutex
	idleTimer *time.Timer
	httpSrv   *http.Server
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

// Serve runs the executor HTTP API on a local listener.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	s.runCtx = ctx
	mux := http.NewServeMux()
	mux.HandleFunc(statusPath, s.handleStatus)
	mux.HandleFunc(submitPath, s.handleSubmit)
	mux.HandleFunc(shutdownPath, s.handleShutdown)

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
	if !s.authorize(w, r) {
		return
	}
	writeProto(w, statusResponse(s.active.Load(), s.version))
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(w, r) {
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

	s.stateMu.Lock()
	if s.shuttingDown {
		s.stateMu.Unlock()
		writeProto(w, &executorpb.SubmitTaskResponse{Accepted: false, Reason: "executor is shutting down"})
		return
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

	writeProto(w, &executorpb.SubmitTaskResponse{Accepted: true})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(w, r) {
		return
	}
	s.stateMu.Lock()
	s.shuttingDown = true
	s.stateMu.Unlock()
	s.stopIdleTimer()
	s.wg.Wait()
	writeProto(w, statusResponse(s.active.Load(), s.version))
	go func() {
		if s.onShutdown != nil {
			s.onShutdown("executor shutdown requested, shutting down")
			return
		}
		_ = s.Stop(context.Background())
	}()
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) bool {
	if s.authToken == "" || r.Header.Get("Authorization") == "Bearer "+s.authToken {
		return true
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
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
