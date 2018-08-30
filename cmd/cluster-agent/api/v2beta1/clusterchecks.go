// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package v2beta1

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
)

// installClusterCheckEndpoints registers v1 clusterchecks endpoints
func installClusterCheckEndpoints(r *mux.Router, sc clusteragent.ServerContext) {
	// TODO
	//r.HandleFunc("/clusterchecks/status/{nodeName}", postCheckStatus).Methods("POST")
	//r.HandleFunc("/clusterchecks/configs/{nodeName}", getCheckConfigs).Methods("GET")

	r.HandleFunc("/clusterchecks", getAllCheckConfigs(sc)).Methods("GET")
}

// getAllCheckConfigs is used by the clustercheck config
func getAllCheckConfigs(sc clusteragent.ServerContext) func(w http.ResponseWriter, r *http.Request) {
	if sc.ClusterCheckHandler == nil {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		configs, err := sc.ClusterCheckHandler.GetAllConfigs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		slcB, err := json.Marshal(configs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(slcB) != 0 {
			w.WriteHeader(http.StatusOK)
			w.Write(slcB)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}
