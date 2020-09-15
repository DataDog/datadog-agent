// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package v1

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	dcautil "github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Install registers v1 API endpoints
func installClusterCheckEndpoints(r *mux.Router, sc clusteragent.ServerContext) {
	r.HandleFunc("/clusterchecks/status/{nodeName}", postCheckStatus(sc)).Methods("POST")
	r.HandleFunc("/clusterchecks/configs/{nodeName}", getCheckConfigs(sc)).Methods("GET")
	r.HandleFunc("/clusterchecks/rebalance", postRebalanceChecks(sc)).Methods("POST")
	r.HandleFunc("/clusterchecks", getState(sc)).Methods("GET")
}

// postCheckStatus is used by the node-agent's config provider
func postCheckStatus(sc clusteragent.ServerContext) func(w http.ResponseWriter, r *http.Request) {
	if sc.ClusterCheckHandler == nil {
		return clusterChecksDisabledHandler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if !shouldHandle(w, r, sc.ClusterCheckHandler, "postCheckStatus") {
			return
		}

		vars := mux.Vars(r)
		nodeName := vars["nodeName"]

		decoder := json.NewDecoder(r.Body)
		var status cctypes.NodeStatus
		err := decoder.Decode(&status)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			incrementRequestMetric("postCheckStatus", http.StatusInternalServerError)
			return
		}

		clientIP, err := validateClientIP(r.Header.Get(dcautil.RealIPHeader))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			incrementRequestMetric("postCheckStatus", http.StatusInternalServerError)
			return
		}

		response, err := sc.ClusterCheckHandler.PostStatus(nodeName, clientIP, status)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			incrementRequestMetric("postCheckStatus", http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, response, "postCheckStatus")
	}
}

// getCheckConfigs is used by the node-agent's config provider
func getCheckConfigs(sc clusteragent.ServerContext) func(w http.ResponseWriter, r *http.Request) {
	if sc.ClusterCheckHandler == nil {
		return clusterChecksDisabledHandler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if !shouldHandle(w, r, sc.ClusterCheckHandler, "getCheckConfigs") {
			return
		}

		vars := mux.Vars(r)
		nodeName := vars["nodeName"]
		response, err := sc.ClusterCheckHandler.GetConfigs(nodeName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			incrementRequestMetric("getCheckConfigs", http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, response, "getCheckConfigs")
	}
}

// postRebalanceChecks requests that the cluster checks be rebalanced
func postRebalanceChecks(sc clusteragent.ServerContext) func(w http.ResponseWriter, r *http.Request) {
	if sc.ClusterCheckHandler == nil {
		return clusterChecksDisabledHandler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if !shouldHandle(w, r, sc.ClusterCheckHandler, "postRebalanceChecks") {
			return
		}

		response, err := sc.ClusterCheckHandler.RebalanceClusterChecks()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			incrementRequestMetric("postRebalanceChecks", http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, response, "postRebalanceChecks")
	}
}

// getState is used by the clustercheck config
func getState(sc clusteragent.ServerContext) func(w http.ResponseWriter, r *http.Request) {
	if sc.ClusterCheckHandler == nil {
		return clusterChecksDisabledHandler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// No redirection for this one, internal endpoint
		response, err := sc.ClusterCheckHandler.GetState()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			incrementRequestMetric("getState", http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, response, "getState")
	}
}

// writeJSONResponse serialises and writes data to the response
func writeJSONResponse(w http.ResponseWriter, data interface{}, handler string) {
	slcB, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		incrementRequestMetric(handler, http.StatusInternalServerError)
		return
	}

	if len(slcB) != 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(slcB)
		incrementRequestMetric(handler, http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	incrementRequestMetric(handler, http.StatusNotFound)
}

// shouldHandle is common code to handle redirection and errors
// due to the handler state
func shouldHandle(w http.ResponseWriter, r *http.Request, h *clusterchecks.Handler, handler string) bool {
	code, reason := h.ShouldHandle()

	switch code {
	case http.StatusOK:
		return true
	case http.StatusFound:
		// Redirection to leader
		url := r.URL
		url.Host = reason
		http.Redirect(w, r, url.String(), http.StatusFound)
		incrementRequestMetric(handler, http.StatusFound)
		return false
	default:
		// Unexpected error
		http.Error(w, reason, code)
		incrementRequestMetric(handler, code)
		return false
	}
}

// clusterChecksDisabledHandler returns a 404 response when cluster-checks are disabled
func clusterChecksDisabledHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusPreconditionFailed)
	w.Write([]byte("Cluster-checks are not enabled"))
}

// validateClientIP validates the http client IP retrieved from the request's header.
// Empty IPs are considered valid for backward compatibility with old clc runner versions
// that don't set the realIPHeader header field.
func validateClientIP(addr string) (string, error) {
	if addr != "" && net.ParseIP(addr) == nil {
		log.Debugf("Error while parsing CLC runner address %s", addr)
		return "", fmt.Errorf("cannot parse CLC runner address: %s", addr)
	}

	if addr == "" && config.Datadog.GetBool("cluster_checks.advanced_dispatching_enabled") {
		log.Warn("Cluster check dispatching error: cannot get runner IP from http headers. advanced_dispatching_enabled requires agent 6.17 or above.")
	}

	return addr, nil
}
