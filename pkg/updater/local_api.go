// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/updater/repository"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	defaultSocketPath = defaultRepositoriesPath + "/updater.sock"
)

// StatusResponse is the response to the status endpoint.
type StatusResponse struct {
	APIResponse
	Version  string                      `json:"version"`
	Packages map[string]repository.State `json:"packages"`
}

// APIResponse is the response to an API request.
type APIResponse struct {
	Error *APIError `json:"error,omitempty"`
}

// APIError is an error response.
type APIError struct {
	Message string `json:"message"`
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
	socketPath := defaultSocketPath
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
func (l *localAPIImpl) Start(_ context.Context) error {
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
	r.HandleFunc("/status", l.status).Methods(http.MethodGet)
	r.HandleFunc("/{package}/experiment/start", l.startExperiment).Methods(http.MethodPost)
	r.HandleFunc("/{package}/experiment/stop", l.stopExperiment).Methods(http.MethodPost)
	r.HandleFunc("/{package}/experiment/promote", l.promoteExperiment).Methods(http.MethodPost)
	r.HandleFunc("/{package}/bootstrap", l.bootstrap).Methods(http.MethodPost)
	return r
}

func (l *localAPIImpl) status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response StatusResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	pacakges, err := l.updater.GetState()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
	response = StatusResponse{
		Version:  version.AgentVersion,
		Packages: pacakges,
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/updater.sock -H 'Content-Type: application/json' http://updater/datadog-agent/experiment/start -d '{"version":"1.21.5"}'
func (l *localAPIImpl) startExperiment(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var request taskWithVersionParams
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Error = &APIError{Message: err.Error()}
		return
	}
	log.Infof("Received local request to start experiment for package %s version %s", pkg, request.Version)
	err = l.updater.StartExperiment(r.Context(), pkg, request.Version)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/updater.sock -H 'Content-Type: application/json' http://updater/datadog-agent/experiment/stop -d '{}'
func (l *localAPIImpl) stopExperiment(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	log.Infof("Received local request to stop experiment for package %s", pkg)
	err := l.updater.StopExperiment(r.Context(), pkg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/updater.sock -H 'Content-Type: application/json' http://updater/datadog-agent/experiment/promote -d '{}'
func (l *localAPIImpl) promoteExperiment(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	log.Infof("Received local request to promote experiment for package %s", pkg)
	err := l.updater.PromoteExperiment(r.Context(), pkg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/updater.sock -H 'Content-Type: application/json' http://updater/datadog-agent/bootstrap -d '{"version":"1.21.5"}'
func (l *localAPIImpl) bootstrap(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var request taskWithVersionParams
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	var err error
	if r.ContentLength > 0 {
		err = json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			response.Error = &APIError{Message: err.Error()}
			return
		}
	}
	if request.Version != "" {
		log.Infof("Received local request to bootstrap package %s version %s", pkg, request.Version)
		err = l.updater.BootstrapVersion(r.Context(), pkg, request.Version)
	} else {
		log.Infof("Received local request to bootstrap package %s", pkg)
		err = l.updater.BootstrapDefault(r.Context(), pkg)

	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// LocalAPIClient is a client to interact with the locally exposed updater API.
type LocalAPIClient interface {
	Status() (StatusResponse, error)

	StartExperiment(pkg, version string) error
	StopExperiment(pkg string) error
	PromoteExperiment(pkg string) error
	BootstrapVersion(pkg, version string) error
}

// LocalAPIClient is a client to interact with the locally exposed updater API.
type localAPIClientImpl struct {
	client *http.Client
	addr   string
}

// NewLocalAPIClient returns a new LocalAPIClient.
func NewLocalAPIClient() LocalAPIClient {
	return &localAPIClientImpl{
		addr: "updater", // this has no meaning when using a unix socket
		client: &http.Client{
			Transport: &http.Transport{
				Dial: func(_, _ string) (net.Conn, error) {
					return net.Dial("unix", defaultSocketPath)
				},
			},
		},
	}
}

// Status returns the status of the updater.
func (c *localAPIClientImpl) Status() (StatusResponse, error) {
	var response StatusResponse
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/status", c.addr), nil)
	if err != nil {
		return response, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return response, err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return response, err
	}
	if response.Error != nil {
		return response, fmt.Errorf("error getting status: %s", response.Error.Message)
	}
	return response, nil
}

// StartExperiment starts an experiment for a package.
func (c *localAPIClientImpl) StartExperiment(pkg, version string) error {
	params := taskWithVersionParams{
		Version: version,
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/%s/experiment/start", c.addr, pkg), bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var response APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("error starting experiment: %s", response.Error.Message)
	}
	return nil
}

// StopExperiment stops an experiment for a package.
func (c *localAPIClientImpl) StopExperiment(pkg string) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/%s/experiment/stop", c.addr, pkg), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var response APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("error stopping experiment: %s", response.Error.Message)
	}
	return nil
}

// PromoteExperiment promotes an experiment for a package.
func (c *localAPIClientImpl) PromoteExperiment(pkg string) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/%s/experiment/promote", c.addr, pkg), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	var response APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("error promoting experiment: %s", response.Error.Message)
	}
	defer resp.Body.Close()
	return nil
}

// BootstrapVersion bootstraps a package to a specific version.
func (c *localAPIClientImpl) BootstrapVersion(pkg, version string) error {
	params := taskWithVersionParams{
		Version: version,
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/%s/bootstrap", c.addr, pkg), bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var response APIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("error starting experiment: %s", response.Error.Message)
	}
	return nil
}
