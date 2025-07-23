// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninsttest

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MockAgentRCServer is a mock remote configuration server that implements
// http.Handler. It pretends to be the agent and serves the config endpoint
// that dd-trace-go uses to fetch the config.
type MockAgentRCServer struct {
	mux       *http.ServeMux
	closeOnce sync.Once
	closeChan chan struct{}
	mu        struct {
		sync.Mutex
		configResp     *core.ClientGetConfigsResponse
		configVersion  int64
		clientVersions map[string]int64
		waiters        map[chan struct{}]struct{}
	}
}

// NewMockAgentRCServer creates a new mock remote-config server.
func NewMockAgentRCServer() *MockAgentRCServer {
	s := &MockAgentRCServer{
		mux:       http.NewServeMux(),
		closeChan: make(chan struct{}),
	}
	s.mu.configResp = &core.ClientGetConfigsResponse{}
	s.mu.clientVersions = make(map[string]int64)
	s.mu.waiters = make(map[chan struct{}]struct{})
	s.mux.HandleFunc("/info", noopHandler)
	s.mux.HandleFunc("/v0.4/traces", noopHandler)
	s.mux.HandleFunc("/v0.7/config", s.handleConfig)

	return s
}

// ServeHTTP makes MockAgentRCServer implement the http.Handler interface.
// It includes middleware to correctly handle request bodies.
func (s *MockAgentRCServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// UpdateRemoteConfig updates the target files for the default /v0.7/config endpoint.
func (s *MockAgentRCServer) UpdateRemoteConfig(
	entries map[string][]byte,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mu.configVersion++
	for c := range s.mu.waiters {
		close(c)
	}
	s.mu.waiters = make(map[chan struct{}]struct{})

	var targetFiles []*core.File
	var clientConfigs []string

	// local structs to build the targets.json
	type fileMeta struct {
		Length int64             `json:"length"`
		Hashes map[string]string `json:"hashes"`
		Custom json.RawMessage   `json:"custom,omitempty"`
	}
	type signedTargets struct {
		Type        string              `json:"_type"`
		SpecVersion string              `json:"spec_version"`
		Version     int64               `json:"version"`
		Expires     string              `json:"expires"`
		Targets     map[string]fileMeta `json:"targets"`
	}

	targetsMap := make(map[string]fileMeta)
	customVersion := json.RawMessage(`{"v": 1}`)

	for path, fileContents := range entries {
		hash := sha256.Sum256(fileContents)
		targetsMap[path] = fileMeta{
			Length: int64(len(fileContents)),
			Hashes: map[string]string{
				"sha256": hex.EncodeToString(hash[:]),
			},
			Custom: customVersion,
		}

		targetFiles = append(targetFiles, &core.File{
			Path: path,
			Raw:  fileContents,
		})
		clientConfigs = append(clientConfigs, path)
	}

	slices.SortFunc(targetFiles, func(i, j *core.File) int {
		return cmp.Compare(i.Path, j.Path)
	})

	targets := signedTargets{
		Type:        "targets",
		SpecVersion: "1.0",
		Version:     time.Now().Unix(),
		Expires:     time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		Targets:     targetsMap,
	}

	signed, err := json.Marshal(targets)
	if err != nil {
		panic(err)
	}

	fakeSignedWrapper := map[string]any{
		"signatures": []string{},
		"signed":     json.RawMessage(signed),
	}
	targetsJSON, err := json.Marshal(fakeSignedWrapper)
	if err != nil {
		panic(err)
	}

	s.mu.configResp = &core.ClientGetConfigsResponse{
		Targets:       targetsJSON,
		TargetFiles:   targetFiles,
		ClientConfigs: clientConfigs,
	}
}

// Close closes the server. It makes all requests return an error.
func (s *MockAgentRCServer) Close() {
	s.closeOnce.Do(func() {
		close(s.closeChan)
	})
}

type pendingConfigRequest struct {
	s        *MockAgentRCServer
	clientID string
	// only one of waitChan and resp will be non-nil
	waitChan chan struct{}
	resp     *core.ClientGetConfigsResponse
}

func (pcr *pendingConfigRequest) getResponse(
	ctx context.Context, maxWait time.Duration,
) (*core.ClientGetConfigsResponse, error) {
	if pcr.resp != nil {
		return pcr.resp, nil
	}
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return pcr.s.getWaitedResponse(pcr.waitChan, pcr.clientID), nil
	case <-pcr.waitChan:
		return pcr.s.getWaitedResponse(pcr.waitChan, pcr.clientID), nil
	case <-pcr.s.closeChan:
		return nil, errors.New("server closed")
	case <-ctx.Done():
		pcr.s.getWaitedResponse(pcr.waitChan, pcr.clientID)
		return nil, ctx.Err()
	}
}

func noopHandler(_ http.ResponseWriter, _ *http.Request) {}

func (s *MockAgentRCServer) getWaitedResponse(
	waitChan chan struct{}, clientID string,
) *core.ClientGetConfigsResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mu.waiters, waitChan)
	s.mu.clientVersions[clientID] = s.mu.configVersion
	return s.mu.configResp
}

func (s *MockAgentRCServer) getPendingConfigRequest(
	clientID string,
) *pendingConfigRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	pcr := &pendingConfigRequest{
		s:        s,
		clientID: clientID,
	}

	lastSeenVersion := s.mu.clientVersions[clientID]
	currentVersion := s.mu.configVersion

	if lastSeenVersion < currentVersion {
		pcr.resp = s.mu.configResp
		s.mu.clientVersions[clientID] = s.mu.configVersion
	} else {
		pcr.waitChan = make(chan struct{})
		s.mu.waiters[pcr.waitChan] = struct{}{}
	}
	return pcr
}

func (s *MockAgentRCServer) writeResponse(w http.ResponseWriter, resp any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(resp); err != nil {
		log.Errorf("failed to marshal response %T: %v", resp, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}

func (s *MockAgentRCServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	var req core.ClientGetConfigsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Errorf("failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var clientID string
	if req.Client == nil || req.Client.ClientTracer == nil {
		log.Errorf("invalid request: missing client tracer")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pcr := s.getPendingConfigRequest(clientID)
	resp, err := pcr.getResponse(r.Context(), time.Second)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	s.writeResponse(w, resp)
}
