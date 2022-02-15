// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/gorilla/mux"
)

// setupConfigHandlers adds the specific handlers for /config endpoints
func setupConfigHandlers(r *mux.Router) {
	r.HandleFunc("/config", settingshttp.Server.GetFull(config.Namespace)).Methods("GET")
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
}
