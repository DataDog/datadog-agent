// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StatusResponse is the response to the status endpoint.
type StatusResponse struct {
	APIResponse
	RemoteConfigState []*pbgo.PackageState `json:"remote_config_state"`
}

// APMInjectionStatus contains the instrumentation status of the APM injection.
type APMInjectionStatus struct {
	HostInstrumented   bool `json:"host_instrumented"`
	DockerInstalled    bool `json:"docker_installed"`
	DockerInstrumented bool `json:"docker_instrumented"`
}

// APIResponse is the response to an API request.
type APIResponse struct {
	Error *APIError `json:"error,omitempty"`
}

// APIError is an error response.
type APIError struct {
	Message string `json:"message"`
}

// LocalAPI is the interface for the locally exposed API to interact with the daemon.
type LocalAPI interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// localAPIImpl is a locally exposed API to interact with the daemon.
type localAPIImpl struct {
	daemon   Daemon
	listener net.Listener
	server   *http.Server
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
	r.HandleFunc("/catalog", l.setCatalog).Methods(http.MethodPost)
	r.HandleFunc("/config_catalog", l.setConfigCatalog).Methods(http.MethodPost)
	r.HandleFunc("/{package}/experiment/start", l.startExperiment).Methods(http.MethodPost)
	r.HandleFunc("/{package}/experiment/stop", l.stopExperiment).Methods(http.MethodPost)
	r.HandleFunc("/{package}/experiment/promote", l.promoteExperiment).Methods(http.MethodPost)
	r.HandleFunc("/{package}/config_experiment/start", l.startConfigExperiment).Methods(http.MethodPost)
	r.HandleFunc("/{package}/config_experiment/stop", l.stopConfigExperiment).Methods(http.MethodPost)
	r.HandleFunc("/{package}/config_experiment/promote", l.promoteConfigExperiment).Methods(http.MethodPost)
	r.HandleFunc("/{package}/install", l.install).Methods(http.MethodPost)
	r.HandleFunc("/{package}/remove", l.remove).Methods(http.MethodPost)
	return r
}

func (l *localAPIImpl) status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response StatusResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	response = StatusResponse{
		RemoteConfigState: l.daemon.GetRemoteConfigState().Packages,
	}
}

func (l *localAPIImpl) setCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var catalog catalog
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	err := json.NewDecoder(r.Body).Decode(&catalog)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Error = &APIError{Message: err.Error()}
		return
	}
	log.Infof("Received local request to set catalog")
	l.daemon.SetCatalog(catalog)
}

func (l *localAPIImpl) setConfigCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var configs map[string]installerConfig
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	err := json.NewDecoder(r.Body).Decode(&configs)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Error = &APIError{Message: err.Error()}
		return
	}
	log.Infof("Received local request to set config catalog")
	l.daemon.SetConfigCatalog(configs)
}

// example: curl -X POST --unix-socket /opt/datadog-packages/run/installer.sock -H 'Content-Type: application/json' http://installer/datadog-agent/experiment/start -d '{"version":"1.21.5"}'
func (l *localAPIImpl) startExperiment(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var request experimentTaskParams
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
	catalogPkg, err := l.daemon.GetPackage(pkg, request.Version)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
	err = l.daemon.StartExperiment(r.Context(), catalogPkg.URL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/run/installer.sock -H 'Content-Type: application/json' http://installer/datadog-agent/experiment/stop -d '{}'
func (l *localAPIImpl) stopExperiment(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	log.Infof("Received local request to stop experiment for package %s", pkg)
	err := l.daemon.StopExperiment(r.Context(), pkg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/run/installer.sock -H 'Content-Type: application/json' http://installer/datadog-agent/experiment/promote -d '{}'
func (l *localAPIImpl) promoteExperiment(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	log.Infof("Received local request to promote experiment for package %s", pkg)
	err := l.daemon.PromoteExperiment(r.Context(), pkg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/run/installer.sock -H 'Content-Type: application/json' http://installer/datadog-agent/config_experiment/start -d '{"version":"1.21.5"}'
func (l *localAPIImpl) startConfigExperiment(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var request experimentTaskParams
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
	err = l.daemon.StartConfigExperiment(r.Context(), pkg, request.Version)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/run/installer.sock -H 'Content-Type: application/json' http://installer/datadog-agent/config_experiment/stop -d '{}'
func (l *localAPIImpl) stopConfigExperiment(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	log.Infof("Received local request to stop experiment for package %s", pkg)
	err := l.daemon.StopConfigExperiment(r.Context(), pkg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/run/installer.sock -H 'Content-Type: application/json' http://installer/datadog-agent/config_experiment/promote -d '{}'
func (l *localAPIImpl) promoteConfigExperiment(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	log.Infof("Received local request to promote experiment for package %s", pkg)
	err := l.daemon.PromoteConfigExperiment(r.Context(), pkg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/run/installer.sock -H 'Content-Type: application/json' http://installer/datadog-agent/install -d '{"version":"1.21.5"}'
func (l *localAPIImpl) install(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var request experimentTaskParams
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

	catalogPkg, err := l.daemon.GetPackage(pkg, request.Version)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}

	log.Infof("Received local request to install package %s version %s", pkg, request.Version)
	err = l.daemon.Install(r.Context(), catalogPkg.URL, request.InstallArgs)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// example: curl -X POST --unix-socket /opt/datadog-packages/run/installer.sock -H 'Content-Type: application/json' http://installer/datadog-agent/remove -d '{}'
func (l *localAPIImpl) remove(w http.ResponseWriter, r *http.Request) {
	pkg := mux.Vars(r)["package"]
	w.Header().Set("Content-Type", "application/json")
	var request experimentTaskParams
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

	log.Infof("Received local request to remove package %s", pkg)
	err = l.daemon.Remove(r.Context(), pkg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}

// LocalAPIClient is a client to interact with the locally exposed daemon API.
type LocalAPIClient interface {
	Status() (StatusResponse, error)

	SetCatalog(catalog string) error
	SetConfigCatalog(configs string) error
	Install(pkg, version string) error
	Remove(pkg string) error
	StartExperiment(pkg, version string) error
	StopExperiment(pkg string) error
	PromoteExperiment(pkg string) error
	StartConfigExperiment(pkg, version string) error
	StopConfigExperiment(pkg string) error
	PromoteConfigExperiment(pkg string) error
}

// LocalAPIClient is a client to interact with the locally exposed daemon API.
type localAPIClientImpl struct {
	client *http.Client
	addr   string
}

// Status returns the status of the daemon.
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

// SetCatalog sets the catalog for the daemon.
func (c *localAPIClientImpl) SetCatalog(catalog string) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/catalog", c.addr), bytes.NewBuffer([]byte(catalog)))
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
		return fmt.Errorf("error setting catalog: %s", response.Error.Message)
	}
	return nil
}

// SetConfigCatalog sets the config catalog for the daemon.
func (c *localAPIClientImpl) SetConfigCatalog(configs string) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/config_catalog", c.addr), bytes.NewBuffer([]byte(configs)))
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
		return fmt.Errorf("error setting config catalog: %s", response.Error.Message)
	}
	return nil
}

// StartExperiment starts an experiment for a package.
func (c *localAPIClientImpl) StartExperiment(pkg, version string) error {
	params := experimentTaskParams{
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

// StartConfigExperiment starts a config experiment for a package.
func (c *localAPIClientImpl) StartConfigExperiment(pkg, version string) error {
	params := experimentTaskParams{
		Version: version,
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/%s/config_experiment/start", c.addr, pkg), bytes.NewBuffer(body))
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
		return fmt.Errorf("error starting config experiment: %s", response.Error.Message)
	}
	return nil
}

// StopConfigExperiment stops an experiment for a package.
func (c *localAPIClientImpl) StopConfigExperiment(pkg string) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/%s/config_experiment/stop", c.addr, pkg), nil)
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
		return fmt.Errorf("error stopping config experiment: %s", response.Error.Message)
	}
	return nil
}

// PromoteConfigExperiment promotes an experiment for a package.
func (c *localAPIClientImpl) PromoteConfigExperiment(pkg string) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/%s/config_experiment/promote", c.addr, pkg), nil)
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
		return fmt.Errorf("error promoting config experiment: %s", response.Error.Message)
	}
	defer resp.Body.Close()
	return nil
}

// Install installs a package with a specific version.
func (c *localAPIClientImpl) Install(pkg, version string) error {
	params := experimentTaskParams{
		Version: version,
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/%s/install", c.addr, pkg), bytes.NewBuffer(body))
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
		return fmt.Errorf("error installing: %s", response.Error.Message)
	}
	return nil
}

// Remove removes a package
func (c *localAPIClientImpl) Remove(pkg string) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/%s/remove", c.addr, pkg), nil)
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
		return fmt.Errorf("error removing: %s", response.Error.Message)
	}
	return nil
}
