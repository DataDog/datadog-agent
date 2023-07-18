// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package v1

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
)

// Install registers v1 API endpoints for endpoints checks
func installEndpointsCheckEndpoints(r *mux.Router, sc clusteragent.ServerContext) {
	r.HandleFunc("/endpointschecks/configs/{nodeName}", api.WithTelemetryWrapper("getEndpointsConfigs", getEndpointsCheckConfigs(sc))).Methods("GET")
	r.HandleFunc("/endpointschecks/configs", api.WithTelemetryWrapper("getAllEndpointsCheckConfigs", getAllEndpointsCheckConfigs(sc))).Methods("GET")
}

// getEndpointsCheckConfigs is used by the node-agent's config provider
func getEndpointsCheckConfigs(sc clusteragent.ServerContext) func(w http.ResponseWriter, r *http.Request) {
	if sc.ClusterCheckHandler == nil {
		return clusterChecksDisabledHandler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if sc.ClusterCheckHandler.RejectOrForwardLeaderQuery(w, r) {
			return
		}

		vars := mux.Vars(r)
		nodeName := vars["nodeName"]
		response, err := sc.ClusterCheckHandler.GetEndpointsConfigs(nodeName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, response)
	}
}

// getAllEndpointsCheckConfigs is used by clusterchecks command to retrieve the endpointscheck configs
func getAllEndpointsCheckConfigs(sc clusteragent.ServerContext) func(w http.ResponseWriter, r *http.Request) {
	if sc.ClusterCheckHandler == nil {
		return clusterChecksDisabledHandler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		response, err := sc.ClusterCheckHandler.GetAllEndpointsCheckConfigs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, response)
	}
}
