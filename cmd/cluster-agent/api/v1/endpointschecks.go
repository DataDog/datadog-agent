// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package v1

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
)

// Install registers v1 API endpoints for endpoints checks
func installEndpointsCheckEndpoints(r *mux.Router, sc clusteragent.ServerContext) {
	r.HandleFunc("/endpointschecks/configs/{nodeName}", getEndpointsCheckConfigs(sc)).Methods("GET")
}

// getEndpointsCheckConfigs is used by the node-agent's config provider
func getEndpointsCheckConfigs(sc clusteragent.ServerContext) func(w http.ResponseWriter, r *http.Request) {
	if sc.ClusterCheckHandler == nil {
		return clusterChecksDisabledHandler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if !shouldHandle(w, r, sc.ClusterCheckHandler, "getEndpointsCheckConfigs") {
			return
		}

		vars := mux.Vars(r)
		nodeName := vars["nodeName"]
		response, err := sc.ClusterCheckHandler.GetEndpointsConfigs(nodeName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			incrementRequestMetric("GetEndpointsConfigs", http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, response, "GetEndpointsConfigs")
	}
}
