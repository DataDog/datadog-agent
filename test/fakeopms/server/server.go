// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package server implements a fake On-Premises Management Service (OPMS) for PAR e2e tests.
// It mimics the endpoints that the Private Action Runner polls, allowing tests to inject
// tasks and capture results without a real Datadog backend.
//
// PAR-facing endpoints (called by the agent):
//   - POST /api/v2/on-prem-management-service/workflow-tasks/dequeue
//   - POST /api/v2/on-prem-management-service/workflow-tasks/publish-task-update
//   - GET  /api/v2/on-prem-management-service/runner/health-check
//   - POST /api/v2/on-prem-management-service/workflow-tasks/heartbeat
//
// Control endpoints (called by the test process):
//   - POST /fakeopms/enqueue
//   - GET  /fakeopms/result
//   - POST /fakeopms/flush
//   - GET  /fakeopms/health
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

const (
	dequeuePath     = "/api/v2/on-prem-management-service/workflow-tasks/dequeue"
	publishPath     = "/api/v2/on-prem-management-service/workflow-tasks/publish-task-update"
	healthCheckPath = "/api/v2/on-prem-management-service/runner/health-check"
	heartbeatPath   = "/api/v2/on-prem-management-service/workflow-tasks/heartbeat"

	enqueueControlPath = "/fakeopms/enqueue"
	resultControlPath  = "/fakeopms/result"
	flushControlPath   = "/fakeopms/flush"
	healthControlPath  = "/fakeopms/health"
)

// QueuedTask is a task to be served to PAR on the next dequeue call.
type QueuedTask struct {
	TaskID    string                 `json:"task_id"`
	ActionFQN string                 `json:"action_fqn"` // e.g. "com.datadoghq.remoteaction.rshell.runCommand"
	Inputs    map[string]interface{} `json:"inputs"`
}

// TaskResult captures what PAR published for a completed task.
type TaskResult struct {
	TaskID       string                 `json:"task_id"`
	Success      bool                   `json:"success"`
	Outputs      map[string]interface{} `json:"outputs,omitempty"`
	ErrorCode    int                    `json:"error_code,omitempty"`
	ErrorDetails string                 `json:"error_details,omitempty"`
}

// Server is the fake OPMS server.
type Server struct {
	mu          sync.Mutex
	queue       []QueuedTask
	results     map[string]*TaskResult
	healthHits  int
	httpServer  http.Server
	ready       chan bool
	url         string
}

// Option is a functional option for Server.
type Option func(*Server)

// WithPort sets the listening port (0 = random available port).
func WithPort(port int) Option {
	return func(s *Server) {
		s.httpServer.Addr = fmt.Sprintf(":%d", port)
	}
}

// WithReadyChannel sets a channel that receives true when the server is ready.
func WithReadyChannel(ready chan bool) Option {
	return func(s *Server) {
		s.ready = ready
	}
}

// NewServer creates a new fake OPMS server.
func NewServer(opts ...Option) *Server {
	s := &Server{
		results: make(map[string]*TaskResult),
		ready:   make(chan bool, 1),
	}
	s.httpServer.Addr = ":8080"

	for _, opt := range opts {
		opt(s)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(dequeuePath, s.handleDequeue)
	mux.HandleFunc(publishPath, s.handlePublishTaskUpdate)
	mux.HandleFunc(healthCheckPath, s.handleHealthCheck)
	mux.HandleFunc(heartbeatPath, s.handleHeartbeat)
	mux.HandleFunc(enqueueControlPath, s.handleEnqueue)
	mux.HandleFunc(resultControlPath, s.handleResult)
	mux.HandleFunc(flushControlPath, s.handleFlush)
	mux.HandleFunc(healthControlPath, s.handleControlHealth)
	s.httpServer.Handler = mux

	return s
}

// Start begins serving in a goroutine. The ready channel receives true when the server is up.
func (s *Server) Start() {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		log.Printf("fakeopms: failed to listen: %v", err)
		if s.ready != nil {
			s.ready <- false
		}
		return
	}
	s.url = fmt.Sprintf("http://%s", ln.Addr().String())
	log.Printf("fakeopms: listening on %s", s.url)
	if s.ready != nil {
		s.ready <- true
	}
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("fakeopms: server error: %v", err)
		}
	}()
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	return s.httpServer.Close()
}

// URL returns the base URL of the server.
func (s *Server) URL() string {
	return s.url
}

// HealthHits returns how many times PAR has called the health-check endpoint.
func (s *Server) HealthHits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healthHits
}

// --- PAR-facing handlers ---

func (s *Server) handleDequeue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.queue) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	task := s.queue[0]
	s.queue = s.queue[1:]

	bundleID, actionName := splitFQN(task.ActionFQN)

	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"id":   task.TaskID,
			"type": "task",
			"attributes": map[string]interface{}{
				"name":      actionName,
				"bundle_id": bundleID,
				"task_id":   task.TaskID,
				"job_id":    task.TaskID, // use same ID; PAR Validate() requires non-empty job_id
				"org_id":    0,
				"inputs":    task.Inputs,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handlePublishTaskUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var req struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				TaskID  string `json:"task_id"`
				JobID   string `json:"job_id"`
				Payload struct {
					// success fields
					Outputs map[string]interface{} `json:"outputs"`
					// failure fields
					ErrorCode    int    `json:"error_code"`
					ErrorDetails string `json:"error_details"`
				} `json:"payload"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	result := &TaskResult{
		TaskID:       req.Data.Attributes.TaskID,
		Success:      req.Data.ID == "succeed_task",
		Outputs:      req.Data.Attributes.Payload.Outputs,
		ErrorCode:    req.Data.Attributes.Payload.ErrorCode,
		ErrorDetails: req.Data.Attributes.Payload.ErrorDetails,
	}

	s.mu.Lock()
	s.results[result.TaskID] = result
	s.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.healthHits++
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"type": "healthCheckResponse",
			"id":   "fake-opms-health",
			"attributes": map[string]interface{}{
				"id": "fake-runner",
			},
		},
	})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// --- Control handlers ---

func (s *Server) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var task QueuedTask
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.queue = append(s.queue, task)
	s.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("taskID")
	if taskID == "" {
		http.Error(w, "taskID query param required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	result, ok := s.results[taskID]
	s.mu.Unlock()

	if !ok {
		http.Error(w, "no result yet", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) handleFlush(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.queue = nil
	s.results = make(map[string]*TaskResult)
	s.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleControlHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// splitFQN splits "com.foo.bar.actionName" into ("com.foo.bar", "actionName").
func splitFQN(fqn string) (string, string) {
	idx := strings.LastIndex(fqn, ".")
	if idx < 0 {
		return fqn, ""
	}
	return fqn[:idx], fqn[idx+1:]
}
