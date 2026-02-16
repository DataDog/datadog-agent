// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config defines the config endpoint of the IPC API Server.
package config

import (
	"errors"
	"expvar"
	"fmt"
	"html"
	"net/http"

	json "github.com/json-iterator/go"

	gorilla "github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	util "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type configEndpoint struct {
	cfg model.Reader

	// runtime metrics about the config endpoint usage
	expvars       *expvar.Map
	successExpvar expvar.Map
	errorsExpvar  expvar.Map
}

func (c *configEndpoint) getConfigValueHandler(w http.ResponseWriter, r *http.Request) {
	vars := gorilla.Vars(r)
	// escape in case it contains html special characters that would be unsafe to include as is in a response
	// all valid config paths won't contain such characters so for a valid request this is a no-op
	path := html.EscapeString(vars["path"])

	if !c.cfg.IsKnown(path) {
		c.errorsExpvar.Add(path, 1)
		log.Warnf("config endpoint received a request from '%s' for config '%s' which does not exist", r.RemoteAddr, path)
		http.Error(w, fmt.Sprintf("config value '%s' does not exist", path), http.StatusNotFound)
		return
	}

	log.Debugf("config endpoint received a request from '%s' for config '%s'", r.RemoteAddr, path)

	var value interface{}
	if path == "logs_config.additional_endpoints" {
		entries, err := encodeInterfaceSliceToStringMap(c.cfg, path)
		if err != nil {
			http.Error(w, fmt.Sprintf("unable to marshal %v: %v", path, err), http.StatusInternalServerError)
			return
		}
		value = entries
	} else {
		value = c.cfg.Get(path)
	}
	c.marshalAndSendResponse(w, path, value)
}

func (c *configEndpoint) getAllConfigValuesHandler(w http.ResponseWriter, r *http.Request) {
	log.Debugf("config endpoint received a request from '%s' for all config values", r.RemoteAddr)
	keys := c.cfg.AllKeysLowercased()
	allValues := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		if key == "logs_config.additional_endpoints" {
			entries, err := encodeInterfaceSliceToStringMap(c.cfg, key)
			if err != nil {
				log.Warnf("error encoding logs_config.additional_endpoints: %v", err)
				continue
			}
			allValues[key] = entries
		} else {
			allValues[key] = c.cfg.Get(key)
		}
	}

	c.marshalAndSendResponse(w, "/", allValues)
}

// GetConfigEndpointMuxCore builds and returns the mux for the config endpoint for the core agent.
// All config keys are readable; access control is enforced by mTLS on the API server.
func GetConfigEndpointMuxCore(cfg model.Reader) *gorilla.Router {
	mux, _ := getConfigEndpoint(cfg, "core")
	return mux
}

// getConfigEndpoint builds and returns the mux and the endpoint state.
func getConfigEndpoint(cfg model.Reader, expvarNamespace string) (*gorilla.Router, *configEndpoint) {
	configEndpoint := &configEndpoint{
		cfg:     cfg,
		expvars: expvar.NewMap(expvarNamespace + "_config_endpoint"),
	}

	for name, expv := range map[string]*expvar.Map{
		"success": &configEndpoint.successExpvar,
		"errors":  &configEndpoint.errorsExpvar,
	} {
		configEndpoint.expvars.Set(name, expv)
	}

	configEndpointMux := gorilla.NewRouter()
	configEndpointMux.HandleFunc("/", http.HandlerFunc(configEndpoint.getAllConfigValuesHandler)).Methods("GET")
	configEndpointMux.HandleFunc("/{path}", http.HandlerFunc(configEndpoint.getConfigValueHandler)).Methods("GET")

	return configEndpointMux, configEndpoint
}

func encodeInterfaceSliceToStringMap(c model.Reader, key string) ([]map[string]string, error) {
	value := c.Get(key)
	if value == nil {
		return nil, nil
	}
	values, ok := value.([]interface{})
	if !ok {
		return nil, errors.New("key does not host a slice of interfaces")
	}

	return util.GetSliceOfStringMap(values)
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
