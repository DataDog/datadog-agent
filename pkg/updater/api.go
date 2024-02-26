// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type startExperimentRequest struct {
	Version string `json:"version"`
}

type startExperimentResponse struct {
	Error string `json:"error,omitempty"`
}

type stopExperimentRequest struct {
}

type stopExperimentResponse struct {
	Error string `json:"error,omitempty"`
}

type promoteExperimentRequest struct {
}

type promoteExperimentResponse struct {
	Error string `json:"error,omitempty"`
}

// LocalAPI is the interface for the locally exposed API to interact with the updater.
type LocalAPI interface {
	Serve() error
	Close() error
}

// localAPIImpl is a locally exposed API to interact with the updater.
type localAPIImpl struct {
	updater  Updater
	listener net.Listener
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
		listener: listener,
		updater:  updater,
	}, nil
}

// Serve serves the LocalAPI.
func (l *localAPIImpl) Serve() error {
	return http.Serve(l.listener, l.handler())
}

// Close closes the LocalAPI.
func (l *localAPIImpl) Close() error {
	return l.listener.Close()
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
	var request startExperimentRequest
	var response startExperimentResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Error = err.Error()
		return
	}
	log.Infof("Received local request to start experiment for package %s version %s", l.updater.GetPackage(), request.Version)
	err = l.updater.StartExperiment(r.Context(), request.Version)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = err.Error()
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/go-updater.sock -H 'Content-Type: application/json' http://agent/experiment/stop -d '{}'
func (l *localAPIImpl) stopExperiment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var request stopExperimentRequest
	var response stopExperimentResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Error = err.Error()
		return
	}
	log.Infof("Received local request to stop experiment for package %s", l.updater.GetPackage())
	err = l.updater.StopExperiment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = err.Error()
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/go-updater.sock -H 'Content-Type: application/json' http://agent/experiment/promote -d '{}'
func (l *localAPIImpl) promoteExperiment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var request promoteExperimentRequest
	var response promoteExperimentResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Error = err.Error()
		return
	}
	log.Infof("Received local request to promote experiment for package %s", l.updater.GetPackage())
	err = l.updater.PromoteExperiment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = err.Error()
		return
	}
}
