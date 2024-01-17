// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config defines the config endpoint of the IPC API Server.
package config

import (
	"encoding/json"
	"expvar"
	"fmt"
	"net/http"

	gorilla "github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type authorizedSet map[string]struct{}

var authorizedConfigPathsCore = authorizedSet{
	"api_key": {},
}

type configEndpoint struct {
	cfg                   config.Reader
	authorizedConfigPaths authorizedSet

	// runtime metrics about the config endpoint usage
	expvars            *expvar.Map
	successExpvar      expvar.Map
	unauthorizedExpvar expvar.Map
	unsetExpvar        expvar.Map
	errorsExpvar       expvar.Map
}

func (c *configEndpoint) serveHTTP(w http.ResponseWriter, r *http.Request) {
	body, statusCode, err := c.getConfigValueAsJSON(r)
	if err != nil {
		http.Error(w, err.Error(), statusCode)
		return
	}

	w.WriteHeader(statusCode)
	_, err = w.Write(body)
	if err != nil {
		log.Warnf("config endpoint: could not write response body: %v", err)
	}
}

// GetConfigEndpointMuxCore builds and returns the mux for the config endpoint with default values
// for the core agent
func GetConfigEndpointMuxCore() *gorilla.Router {
	return GetConfigEndpointMux(config.Datadog, authorizedConfigPathsCore, "core")
}

// GetConfigEndpointMux builds and returns the mux for the config endpoint, with the given config,
// authorized paths, and expvar namespace
func GetConfigEndpointMux(cfg config.Reader, authorizedConfigPaths authorizedSet, expvarNamespace string) *gorilla.Router {
	mux, _ := getConfigEndpoint(cfg, authorizedConfigPaths, expvarNamespace)
	return mux
}

// getConfigEndpoint builds and returns the mux and the endpoint state.
func getConfigEndpoint(cfg config.Reader, authorizedConfigPaths authorizedSet, expvarNamespace string) (*gorilla.Router, *configEndpoint) {
	configEndpoint := &configEndpoint{
		cfg:                   cfg,
		authorizedConfigPaths: authorizedConfigPaths,
		expvars:               expvar.NewMap(expvarNamespace + "_config_endpoint"),
	}

	for name, expv := range map[string]*expvar.Map{
		"success":      &configEndpoint.successExpvar,
		"unauthorized": &configEndpoint.unauthorizedExpvar,
		"unset":        &configEndpoint.unsetExpvar,
		"errors":       &configEndpoint.errorsExpvar,
	} {
		configEndpoint.expvars.Set(name, expv)
	}

	configEndpointHandler := http.HandlerFunc(configEndpoint.serveHTTP)
	configEndpointMux := gorilla.NewRouter()
	configEndpointMux.HandleFunc("/", configEndpointHandler).Methods("GET")
	configEndpointMux.HandleFunc("/{path}", configEndpointHandler).Methods("GET")

	return configEndpointMux, configEndpoint
}

// returns the marshalled JSON value of the config path requested
// or an error and http status code in case of failure
func (c *configEndpoint) getConfigValueAsJSON(r *http.Request) ([]byte, int, error) {
	vars := gorilla.Vars(r)
	path := vars["path"]

	if _, ok := c.authorizedConfigPaths[path]; !ok {
		c.unauthorizedExpvar.Add(path, 1)
		log.Warnf("config endpoint received a request from '%s' for config '%s' which is not allowed", r.RemoteAddr, path)
		return nil, http.StatusForbidden, fmt.Errorf("querying config value '%s' is not allowed", path)
	}

	log.Debug("config endpoint received a request from '%s' for config '%s'", r.RemoteAddr, path)
	value := c.cfg.Get(path)
	if value == nil {
		c.unsetExpvar.Add(path, 1)
		return nil, http.StatusNotFound, fmt.Errorf("no runtime setting found for %s", path)
	}

	body, err := json.Marshal(value)
	if err != nil {
		c.errorsExpvar.Add(path, 1)
		return nil, http.StatusInternalServerError, fmt.Errorf("could not marshal config value of '%s': %v", path, err)
	}

	c.successExpvar.Add(path, 1)
	return body, http.StatusOK, nil
}
