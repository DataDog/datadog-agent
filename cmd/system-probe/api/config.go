// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/core/settings"
)

// setupConfigHandlers adds the specific handlers for /config endpoints
func setupConfigHandlers(r *mux.Router, settings settings.Component) {
	r.HandleFunc("/config", settings.GetFullConfig(getAggregatedNamespaces()...)).Methods("GET")
	r.HandleFunc("/config/list-runtime", settings.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		setting := vars["setting"]
		settings.GetValue(setting, w, r)
	}).Methods("GET")
	r.HandleFunc("/config/{setting}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		setting := vars["setting"]
		settings.SetValue(setting, w, r)
	}).Methods("POST")
}

func getAggregatedNamespaces() []string {
	namespaces := []string{
		config.Namespace,
	}
	for _, m := range modules.All {
		namespaces = append(namespaces, m.ConfigNamespaces...)
	}
	return namespaces
}
