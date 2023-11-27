// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
)

type APIServerDeps struct {
	fx.In

	Config       config.Component
	Log          log.Component
	WorkloadMeta workloadmeta.Component
}

func injectDeps(deps APIServerDeps, handler func(APIServerDeps, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		handler(deps, writer, req)
	}
}

func SetupAPIServerHandlers(deps APIServerDeps, r *mux.Router) {
	r.HandleFunc("/config", settingshttp.Server.GetFullDatadogConfig("process_config")).Methods("GET") // Get only settings in the process_config namespace
	r.HandleFunc("/config/all", settingshttp.Server.GetFullDatadogConfig("")).Methods("GET")           // Get all fields from process-agent Config object
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
	r.HandleFunc("/agent/status", injectDeps(deps, statusHandler)).Methods("GET")
	r.HandleFunc("/agent/tagger-list", injectDeps(deps, getTaggerList)).Methods("GET")
	r.HandleFunc("/agent/workload-list/short", func(w http.ResponseWriter, r *http.Request) {
		workloadList(w, false, deps.WorkloadMeta)
	}).Methods("GET")
	r.HandleFunc("/agent/workload-list/verbose", func(w http.ResponseWriter, r *http.Request) {
		workloadList(w, true, deps.WorkloadMeta)
	}).Methods("GET")
	r.HandleFunc("/check/{check}", checkHandler).Methods("GET")
}
