// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func setupHandlers(r *mux.Router) {
	r.HandleFunc("/config", settingshttp.Server.GetFullDatadogConfig("process_config")).Methods("GET") // Get only settings in the process_config namespace
	r.HandleFunc("/config/all", settingshttp.Server.GetFullDatadogConfig("")).Methods("GET")           // Get all fields from process-agent Config object
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
	r.HandleFunc("/agent/status", statusHandler).Methods("GET")
	r.HandleFunc("/agent/tagger-list", getTaggerList).Methods("GET")
	r.HandleFunc("/agent/workload-list/short", getShortWorkloadList).Methods("GET")
	r.HandleFunc("/agent/workload-list/verbose", getVerboseWorkloadList).Methods("GET")
	r.HandleFunc("/check/{check}", checkHandler).Methods("GET")
}

// StartServer starts the config server
func StartServer() error {
	// Set up routes
	r := mux.NewRouter()
	setupHandlers(r)

	addr, err := ddconfig.GetProcessAPIAddressPort()
	if err != nil {
		return err
	}
	log.Infof("API server listening on %s", addr)
	timeout := time.Duration(ddconfig.Datadog.GetInt("server_timeout")) * time.Second
	srv := &http.Server{
		Handler:      r,
		Addr:         addr,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		IdleTimeout:  timeout,
	}

	go func() {
		err := srv.ListenAndServe()
		if err != nil {
			_ = log.Error(err)
		}
	}()
	return nil
}
