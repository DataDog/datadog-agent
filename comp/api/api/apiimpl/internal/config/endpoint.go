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
	"html"
	"net/http"
	"strings"

	gorilla "github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const prefixPathSuffix string = "."

type authorizedSet map[string]struct{}

var authorizedConfigPathsCore = buildAuthorizedSet(
	"api_key", "site", "dd_url", "logs_config", "ha",
)

func buildAuthorizedSet(paths ...string) authorizedSet {
	authorizedPaths := make(authorizedSet)
	for _, path := range paths {
		authorizedPaths[path] = struct{}{}
	}
	return authorizedPaths
}

type configEndpoint struct {
	cfg                   config.Reader
	authorizedConfigPaths authorizedSet

	// runtime metrics about the config endpoint usage
	expvars            *expvar.Map
	successExpvar      expvar.Map
	unauthorizedExpvar expvar.Map
	errorsExpvar       expvar.Map
}

func (c *configEndpoint) getConfigValueHandler(w http.ResponseWriter, r *http.Request) {
	vars := gorilla.Vars(r)
	// escape in case it contains html special characters that would be unsafe to include as is in a response
	// all valid config paths won't contain such characters so for a valid request this is a no-op
	path := html.EscapeString(vars["path"])

	authorized := false
	if _, ok := c.authorizedConfigPaths[path]; ok {
		authorized = true
	} else {
		// check to see if the requested path matches any of the authorized paths by trying to treat
		// the authorized path as a prefix: if the requested path is `foo.bar` and we have an
		// authorized path of `foo`, then `foo.bar` would be allowed, or if we had a requested path
		// of `foo.bar.quux`, and an authorized path of `foo.bar`, it would also be allowed
		for authorizedPath := range c.authorizedConfigPaths {
			if strings.HasPrefix(path, authorizedPath+prefixPathSuffix) {
				authorized = true
				break
			}
		}
	}

	if !authorized {
		c.unauthorizedExpvar.Add(path, 1)
		log.Warnf("config endpoint received a request from '%s' for config '%s' which is not allowed", r.RemoteAddr, path)
		http.Error(w, fmt.Sprintf("querying config value '%s' is not allowed", path), http.StatusForbidden)
		return
	}

	if !c.cfg.IsKnown(path) {
		c.errorsExpvar.Add(path, 1)
		log.Warnf("config endpoint received a request from '%s' for config '%s' which does not exist", r.RemoteAddr, path)
		http.Error(w, fmt.Sprintf("config value '%s' does not exist", path), http.StatusNotFound)
		return
	}

	log.Debug("config endpoint received a request from '%s' for config '%s'", r.RemoteAddr, path)
	value := c.cfg.Get(path)
	c.marshalAndSendResponse(w, path, value)
}

func (c *configEndpoint) getAllConfigValuesHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug("config endpoint received a request from '%s' for all authorized config values", r.RemoteAddr)
	allValues := make(map[string]interface{}, len(c.authorizedConfigPaths))
	for key := range c.authorizedConfigPaths {
		allValues[key] = c.cfg.Get(key)
	}

	c.marshalAndSendResponse(w, "/", allValues)
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
		"errors":       &configEndpoint.errorsExpvar,
	} {
		configEndpoint.expvars.Set(name, expv)
	}

	configEndpointMux := gorilla.NewRouter()
	configEndpointMux.HandleFunc("/", http.HandlerFunc(configEndpoint.getAllConfigValuesHandler)).Methods("GET")
	configEndpointMux.HandleFunc("/{path}", http.HandlerFunc(configEndpoint.getConfigValueHandler)).Methods("GET")

	return configEndpointMux, configEndpoint
}

func (c *configEndpoint) marshalAndSendResponse(w http.ResponseWriter, path string, value interface{}) {
	body, err := json.Marshal(value)
	if err != nil {
		c.errorsExpvar.Add(path, 1)
		http.Error(w, fmt.Sprintf("could not marshal config value of '%s': %v", path, err), http.StatusInternalServerError)
		return
	}

	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	if err != nil {
		c.errorsExpvar.Add(path, 1)
		log.Warnf("config endpoint: could not write response body: %v", err)
		return
	}
	c.successExpvar.Add(path, 1)
}
