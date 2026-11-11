// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/middleware"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	restrictedSocketName  = "agent-deps.sock"
	defaultDepPackageName = "datadog-agent-integration-deps"
)

// restrictedLocalAPIImpl exposes a narrow install/remove endpoint for the
// on-demand python dependency package on a unix socket that the agent user
// can reach. Only the package named by on_demand_python_deps.dep_package_name
// may be installed or removed through it.
type restrictedLocalAPIImpl struct {
	daemon   Daemon
	config   agentconfig.Reader
	listener net.Listener
	server   *http.Server
}

// NewRestrictedLocalAPI returns a LocalAPI exposing the restricted dependency
// install/remove endpoint, served from a unix socket in the installer run
// path with permissions allowing the dd-agent user. It returns a no-op
// implementation when on_demand_python_deps is not enabled.
func NewRestrictedLocalAPI(daemon Daemon, config agentconfig.Reader) (LocalAPI, error) {
	if !config.GetBool("on_demand_python_deps.enabled") {
		return nil, nil
	}
	socketPath := filepath.Join(paths.RunPath, restrictedSocketName)
	if err := os.RemoveAll(socketPath); err != nil {
		return nil, fmt.Errorf("could not remove socket: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o770); err != nil {
		return nil, fmt.Errorf("error setting socket permissions: %v", err)
	}
	if group, err := user.LookupGroup("dd-agent"); err == nil {
		if gid, err := strconv.Atoi(group.Gid); err == nil {
			if err := os.Chown(socketPath, -1, gid); err != nil {
				log.Warnf("could not chown restricted installer socket to dd-agent: %v", err)
			}
		}
	} else {
		log.Warnf("could not lookup dd-agent group, restricted installer socket may not be reachable by the agent: %v", err)
	}

	impl := &restrictedLocalAPIImpl{
		daemon:   daemon,
		config:   config,
		listener: listener,
		server:   &http.Server{},
	}
	impl.server.Handler = impl.handler()
	return impl, nil
}

// Start starts the restricted LocalAPI.
func (l *restrictedLocalAPIImpl) Start(_ context.Context) error {
	go func() {
		err := l.server.Serve(l.listener)
		if err != nil {
			log.Infof("Restricted local API server stopped: %v", err)
		}
	}()
	log.Infof("Installer restricted dependency API listening on %s", restrictedSocketName)
	return nil
}

// Stop stops the restricted LocalAPI.
func (l *restrictedLocalAPIImpl) Stop(ctx context.Context) error {
	return l.server.Shutdown(ctx)
}

func (l *restrictedLocalAPIImpl) handler() http.Handler {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("POST /{package}/install", l.install)
	apiMux.HandleFunc("POST /{package}/remove", l.remove)

	r := http.NewServeMux()
	r.Handle("/", middleware.RequireContentType("application/json")(apiMux))
	return r
}

func (l *restrictedLocalAPIImpl) allowedPackage(pkg string) bool {
	allowed := l.config.GetString("on_demand_python_deps.dep_package_name")
	if allowed == "" {
		allowed = defaultDepPackageName
	}
	return pkg == allowed
}

func (l *restrictedLocalAPIImpl) install(w http.ResponseWriter, r *http.Request) {
	pkg := r.PathValue("package")
	w.Header().Set("Content-Type", "application/json")
	var request experimentTaskParams
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	if !l.allowedPackage(pkg) {
		w.WriteHeader(http.StatusForbidden)
		response.Error = &APIError{Message: fmt.Sprintf("package %s is not an allowed dependency package", pkg)}
		return
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
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
	log.Infof("Received restricted local request to install package %s version %s", pkg, request.Version)
	// The install runs detached from the request: the collector notices
	// completion by re-probing its deferred checks, and a long download must
	// not be cancelled by a client disconnect.
	url := catalogPkg.URL
	go func() {
		if err := l.daemon.Install(context.WithoutCancel(r.Context()), url, nil); err != nil {
			log.Errorf("Restricted install of package %s failed: %v", pkg, err)
		}
	}()
}

func (l *restrictedLocalAPIImpl) remove(w http.ResponseWriter, r *http.Request) {
	pkg := r.PathValue("package")
	w.Header().Set("Content-Type", "application/json")
	var response APIResponse
	defer func() {
		_ = json.NewEncoder(w).Encode(response)
	}()
	if !l.allowedPackage(pkg) {
		w.WriteHeader(http.StatusForbidden)
		response.Error = &APIError{Message: fmt.Sprintf("package %s is not an allowed dependency package", pkg)}
		return
	}
	log.Infof("Received restricted local request to remove package %s", pkg)
	if err := l.daemon.Remove(r.Context(), pkg); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Error = &APIError{Message: err.Error()}
		return
	}
}
