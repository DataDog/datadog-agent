// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
)

// setupConfigHandlers adds the specific handlers for /config endpoints
func setupConfigHandlers(r *mux.Router) {
	r.HandleFunc("/config", settingshttp.Server.GetFullSystemProbeConfig(getAggregatedNamespaces()...)).Methods("GET")
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
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
