// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type startExperimentParams struct {
	Version string `json:"version"`
}

type apiResponse struct {
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
}

const (
	methodStartExperiment   = "start_experiment"
	methodStopExperiment    = "stop_experiment"
	methodPromoteExperiment = "promote_experiment"
)

func handleRequest(ctx context.Context, u Updater, method string, rawParams []byte) error {
	switch method {
	case methodStartExperiment:
		var params startExperimentParams
		err := json.Unmarshal(rawParams, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		return u.StartExperiment(ctx, params.Version)
	case methodStopExperiment:
		return u.StopExperiment()
	case methodPromoteExperiment:
		return u.PromoteExperiment()
	default:
		return fmt.Errorf("unknown method: %s", method)
	}
}

type expectedState struct {
	Stable     string `json:"stable"`
	Experiment string `json:"experiment"`
}

type remoteApiRequest struct {
	ID            string          `json:"id"`
	ExpectedState expectedState   `json:"expected_state"`
	Method        string          `json:"method"`
	Params        json.RawMessage `json:"params"`
}

type remoteAPI struct {
	executedRequests map[string]struct{}
}

func newRemoteAPI() *remoteAPI {
	return &remoteAPI{
		executedRequests: make(map[string]struct{}),
	}
}

func (r *remoteAPI) handleRequests(u Updater, requestConfigs map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) error {
	for id, requestConfig := range requestConfigs {
		var request remoteApiRequest
		err := json.Unmarshal(requestConfig.Config, &request)
		if err != nil {
			return fmt.Errorf("could not unmarshal request: %w", err)
		}
		if _, ok := r.executedRequests[request.ID]; ok {
			log.Debugf("request %s already executed", request.ID)
			continue
		}
		s, err := u.GetState()
		if err != nil {
			log.Errorf("could not get updater state: %s", err)
			return err
		}
		if s.Stable != request.ExpectedState.Stable || s.Experiment != request.ExpectedState.Experiment {
			log.Debugf("request %s not executed: state does not match: expected %v, got %v", request.ID, request.ExpectedState, s)
		}
		r.executedRequests[request.ID] = struct{}{}
		err = handleRequest(context.Background(), u, request.Method, request.Params)
		if err != nil {
			apiErr := apiError{Message: err.Error()}
			rawAPIErr, err := json.Marshal(apiErr)
			if err != nil {
				return fmt.Errorf("could not marshal api error: %w", err)
			}
			applyStateCallback(id, state.ApplyStatus{State: state.ApplyStateError, Error: string(rawAPIErr)})
			continue
		}
		applyStateCallback(id, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	return nil
}

// LocalAPI is the interface for the locally exposed API to interact with the updater.
type LocalAPI interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// localAPIImpl is a locally exposed API to interact with the updater.
type localAPIImpl struct {
	updater  Updater
	listener net.Listener
	server   *http.Server
}

// NewLocalAPI returns a new LocalAPI.
func NewLocalAPI(updater Updater) (LocalAPI, error) {
	socketPath := path.Join(updater.GetRepositoryPath(), fmt.Sprintf("%s-updater.sock", updater.GetPackage()))
	err := os.RemoveAll(socketPath)
	if err != nil {
		return nil, fmt.Errorf("could not remove socket: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0700); err != nil {
		return nil, fmt.Errorf("error setting socket permissions: %v", err)
	}
	return &localAPIImpl{
		server:   &http.Server{},
		listener: listener,
		updater:  updater,
	}, nil
}

// Start starts the LocalAPI.
func (l *localAPIImpl) Start(ctx context.Context) error {
	l.server.Handler = l.handler()
	go func() {
		err := l.server.Serve(l.listener)
		if err != nil {
			log.Infof("Local API server stopped: %v", err)
		}
	}()
	return nil
}

// Stop stops the LocalAPI.
func (l *localAPIImpl) Stop(ctx context.Context) error {
	return l.server.Shutdown(ctx)
}

func (l *localAPIImpl) handler() http.Handler {
	r := mux.NewRouter().Headers("Content-Type", "application/json").Subrouter()
	r.HandleFunc("/experiment/start", l.startExperiment).Methods(http.MethodPost)
	r.HandleFunc("/experiment/stop", l.stopExperiment).Methods(http.MethodPost)
	r.HandleFunc("/experiment/promote", l.promoteExperiment).Methods(http.MethodPost)
	return r
}

// example: curl -X POST --unix-socket /opt/datadog-packages/go-updater.sock -H 'Content-Type: application/json' http://agent/experiment/start -d '{"version":"1.21.5"}'
func (l *localAPIImpl) startExperiment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var request startExperimentParams
	var response apiResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Error = &apiError{Message: err.Error()}
		return
	}
	log.Infof("Received local request to start experiment for package %s version %s", l.updater.GetPackage(), request.Version)
	err = l.updater.StartExperiment(r.Context(), request.Version)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &apiError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/go-updater.sock -H 'Content-Type: application/json' http://agent/experiment/stop -d '{}'
func (l *localAPIImpl) stopExperiment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response apiResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	log.Infof("Received local request to stop experiment for package %s", l.updater.GetPackage())
	err := l.updater.StopExperiment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &apiError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/go-updater.sock -H 'Content-Type: application/json' http://agent/experiment/promote -d '{}'
func (l *localAPIImpl) promoteExperiment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response apiResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	log.Infof("Received local request to promote experiment for package %s", l.updater.GetPackage())
	err := l.updater.PromoteExperiment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &apiError{Message: err.Error()}
		return
	}
}
