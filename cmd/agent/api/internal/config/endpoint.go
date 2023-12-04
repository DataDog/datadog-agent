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

var authorizedConfigPaths = authorizedSet{
	"api_key": {},
}

// runtime metrics about the config endpoint usage
var (
	successExpvar      *expvar.Map
	unauthorizedExpvar *expvar.Map
	unsetExpvar        *expvar.Map
	failedExpvar       *expvar.Map
	expvars            *expvar.Map
)

func init() {
	expvars = expvar.NewMap("config_endpoint")
	for name, expv := range map[string]**expvar.Map{
		"success":      &successExpvar,
		"unauthorized": &unauthorizedExpvar,
		"unset":        &unsetExpvar,
		"failed":       &failedExpvar,
	} {
		(*expv) = expvar.NewMap(name)
		expvars.Set(name, *expv)
	}
}

// GetConfigEndpointMux builds and returns the mux for the config endpoint
func GetConfigEndpointMux(cfg config.Reader) *gorilla.Router {
	return getConfigEndpointMux(cfg, authorizedConfigPaths)
}

func getConfigEndpointMux(cfg config.Reader, authorizedConfigPaths authorizedSet) *gorilla.Router {
	configEndpointHandler := func(w http.ResponseWriter, r *http.Request) {
		body, statusCode, err := getConfigValueAsJSON(cfg, r, authorizedConfigPaths)
		if err != nil {
			http.Error(w, err.Error(), statusCode)
			return
		}

		w.WriteHeader(statusCode)
		_, _ = w.Write(body)
	}

	configEndpointMux := gorilla.NewRouter()
	configEndpointMux.HandleFunc("/", configEndpointHandler).Methods("GET")
	configEndpointMux.HandleFunc("/{path}", configEndpointHandler).Methods("GET")

	return configEndpointMux
}

// returns the marshalled JSON value of the config path requested
// or an error and http status code in case of failure
func getConfigValueAsJSON(cfg config.Reader, r *http.Request, authorizedConfigPaths authorizedSet) ([]byte, int, error) {
	vars := gorilla.Vars(r)
	path := vars["path"]

	if _, ok := authorizedConfigPaths[path]; !ok {
		unauthorizedExpvar.Add(path, 1)
		log.Warnf("config endpoint received a request from '%s' for config '%s' which is not allowed", r.RemoteAddr, path)
		return nil, http.StatusForbidden, fmt.Errorf("querying config value '%s' is not allowed", path)
	}

	log.Debug("config endpoint received a request from '%s' for config '%s'", r.RemoteAddr, path)
	value := cfg.Get(path)
	if value == nil {
		unsetExpvar.Add(path, 1)
		return nil, http.StatusNotFound, fmt.Errorf("no runtime setting found for %s", path)
	}

	body, err := json.Marshal(value)
	if err != nil {
		failedExpvar.Add(path, 1)
		return nil, http.StatusInternalServerError, fmt.Errorf("could not marshal config value of '%s': %v", path, err)
	}

	successExpvar.Add(path, 1)
	return body, http.StatusOK, nil
}
