// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Command scenario-service is a stub long-running service that builds and drives
// scenariorun binaries from caller-specified commits. It demonstrates the
// commit -> build -> execute loop; production concerns (auth, persistence,
// async jobs, build farm) are intentionally out of scope.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
)

type runRequest struct {
	Commit   string            `json:"commit"`
	Scenario string            `json:"scenario"`
	Config   map[string]string `json:"config"`
}

type actionRequest struct {
	Config map[string]string `json:"config"`
}

type runRecord struct {
	ID       string `json:"run_id"`
	Stack    string `json:"stack_id"`
	Commit   string `json:"commit"`
	Scenario string `json:"scenario"`
	Status   string `json:"status"`
}

type server struct {
	driver Driver
	mu     sync.Mutex
	runs   map[string]*runRecord
	seq    int
}

func (s *server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Commit == "" || req.Scenario == "" {
		http.Error(w, "commit and scenario are required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("run-%d", s.seq)
	stack := fmt.Sprintf("%s-%s", req.Scenario, id)
	rec := &runRecord{ID: id, Stack: stack, Commit: req.Commit, Scenario: req.Scenario, Status: "provisioning"}
	s.runs[id] = rec
	s.mu.Unlock()

	if err := s.driver.Run(req.Commit, req.Scenario, stack, req.Config); err != nil {
		s.mu.Lock()
		rec.Status = "failed"
		s.mu.Unlock()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	rec.Status = "running"
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, rec)
}

func (s *server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	rec, ok := s.runs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *server) handleRunAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	action := r.PathValue("action")
	s.mu.Lock()
	rec, ok := s.runs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	var req actionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.driver.Action(rec.Commit, rec.Scenario, action, rec.Stack, req.Config); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	rec, ok := s.runs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if err := s.driver.Destroy(rec.Commit, rec.Scenario, rec.Stack); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	rec.Status = "destroyed"
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, rec)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	addr := os.Getenv("SCENARIO_SERVICE_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	repoRoot := os.Getenv("SCENARIO_SERVICE_REPO")
	if repoRoot == "" {
		repoRoot = "."
	}
	cacheDir, err := os.MkdirTemp("", "scenario-service-cache-")
	if err != nil {
		log.Fatalf("failed to create cache dir: %v", err)
	}
	s := &server{
		driver: Driver{Builder: newGitBuilder(repoRoot, cacheDir)},
		runs:   map[string]*runRecord{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /runs", s.handleCreateRun)
	mux.HandleFunc("GET /runs/{id}", s.handleGetRun)
	mux.HandleFunc("POST /runs/{id}/actions/{action}", s.handleRunAction)
	mux.HandleFunc("DELETE /runs/{id}", s.handleDeleteRun)

	log.Printf("scenario-service listening on %s (repo=%s)", addr, repoRoot)
	log.Fatal(http.ListenAndServe(addr, mux)) //nolint:gosec
}
