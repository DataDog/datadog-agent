// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
)

// parServerState holds the in-memory task queue and result map for PAR e2e tests.
// The Private Action Runner polls /api/v2/on-prem-management-service/workflow-tasks/dequeue
// to receive tasks; tests use /fakeintake/par/* control endpoints to enqueue tasks and
// read back results without needing a real OPMS backend.
type parServerState struct {
	mu           sync.Mutex
	queue        []parQueuedTask
	results      map[string]*PARTaskResult
	dequeueCalls int // counts how many times PAR has called the dequeue endpoint
}

type parQueuedTask struct {
	TaskID    string                 `json:"task_id"`
	ActionFQN string                 `json:"action_fqn"`
	Inputs    map[string]interface{} `json:"inputs"`
}

// PARTaskResult captures what PAR published for a completed task.
type PARTaskResult struct {
	TaskID       string                 `json:"task_id"`
	Success      bool                   `json:"success"`
	Outputs      map[string]interface{} `json:"outputs,omitempty"`
	ErrorCode    int                    `json:"error_code,omitempty"`
	ErrorDetails string                 `json:"error_details,omitempty"`
}

// --- PAR-facing handlers (called by the agent) ---

func (fi *Server) handlePARDequeue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fi.par.mu.Lock()
	defer fi.par.mu.Unlock()

	fi.par.dequeueCalls++

	if len(fi.par.queue) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	task := fi.par.queue[0]
	fi.par.queue = fi.par.queue[1:]

	bundleID, actionName := parSplitFQN(task.ActionFQN)
	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"id":   task.TaskID,
			"type": "task",
			"attributes": map[string]interface{}{
				"name":      actionName,
				"bundle_id": bundleID,
				"task_id":   task.TaskID,
				"job_id":    task.TaskID,
				"org_id":    0,
				"inputs":    task.Inputs,
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (fi *Server) handlePARPublish(w http.ResponseWriter, r *http.Request) {
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
				Payload struct {
					Outputs      map[string]interface{} `json:"outputs"`
					ErrorCode    int                    `json:"error_code"`
					ErrorDetails string                 `json:"error_details"`
				} `json:"payload"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	result := &PARTaskResult{
		TaskID:       req.Data.Attributes.TaskID,
		Success:      req.Data.ID == "succeed_task",
		Outputs:      req.Data.Attributes.Payload.Outputs,
		ErrorCode:    req.Data.Attributes.Payload.ErrorCode,
		ErrorDetails: req.Data.Attributes.Payload.ErrorDetails,
	}

	fi.par.mu.Lock()
	fi.par.results[result.TaskID] = result
	fi.par.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (fi *Server) handlePARHealthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"type": "healthCheckResponse",
			"id":   "fakeintake-par",
			"attributes": map[string]interface{}{
				"id": "fake-runner",
			},
		},
	})
}

func (fi *Server) handlePARHeartbeat(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// --- Control handlers (called by the test process) ---

func (fi *Server) handlePAREnqueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var task parQueuedTask
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	fi.par.mu.Lock()
	fi.par.queue = append(fi.par.queue, task)
	fi.par.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (fi *Server) handlePARResult(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("taskID")
	if taskID == "" {
		http.Error(w, "taskID query param required", http.StatusBadRequest)
		return
	}

	fi.par.mu.Lock()
	result, ok := fi.par.results[taskID]
	fi.par.mu.Unlock()

	if !ok {
		http.Error(w, "no result yet", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (fi *Server) handlePARFlush(w http.ResponseWriter, _ *http.Request) {
	fi.par.mu.Lock()
	fi.par.queue = nil
	fi.par.results = make(map[string]*PARTaskResult)
	fi.par.dequeueCalls = 0
	fi.par.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (fi *Server) handlePARStats(w http.ResponseWriter, _ *http.Request) {
	fi.par.mu.Lock()
	calls := fi.par.dequeueCalls
	fi.par.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"dequeue_calls": calls})
}

// parSplitFQN splits "com.foo.bar.actionName" into ("com.foo.bar", "actionName").
func parSplitFQN(fqn string) (string, string) {
	idx := strings.LastIndex(fqn, ".")
	if idx < 0 {
		return fqn, ""
	}
	return fqn[:idx], fqn[idx+1:]
}
