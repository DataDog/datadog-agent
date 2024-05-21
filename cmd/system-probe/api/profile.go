// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// setupProfilingHandlers adds the specific handlers for a few profiling endpoints
func setupProfilingHandlers(r *mux.Router, settings settings.Component, sysprobeconfig ddconfig.Reader, configPrefix string, log log.Component) {
	// Register pprof handlers
	r.PathPrefix("/debug/pprof").Handler(http.DefaultServeMux)

	// Register internal profiling handler
	r.HandleFunc("/internal-profile/{cmd}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		cmd := vars["cmd"]

		var err error
		if cmd == "start" {
			err = startProfiler(settings, sysprobeconfig, configPrefix, log)
		} else if cmd == "stop" {
			stopProfiler(settings, sysprobeconfig, configPrefix, log)
		} else {
			err = fmt.Errorf("Invalid command %s", cmd)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}).Methods("GET")
}

func startProfiler(settings settings.Component, sysprobeconfig ddconfig.Reader, configPrefix string, log log.Component) error {
	if v := sysprobeconfig.GetInt(configPrefix + "internal_profiling.block_profile_rate"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_block_profile_rate", v, model.SourceAgentRuntime); err != nil {
			log.Errorf("Error setting block profile rate: %v", err)
		}
	}

	if v := sysprobeconfig.GetInt(configPrefix + "internal_profiling.mutex_profile_fraction"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_mutex_profile_fraction", v, model.SourceAgentRuntime); err != nil {
			log.Errorf("Error mutex profile fraction: %v", err)
		}
	}

	err := settings.SetRuntimeSetting("internal_profiling", true, model.SourceAgentRuntime)
	if err != nil {
		log.Errorf("Error starting profiler: %v", err)
	}

	return err
}

func stopProfiler(settings settings.Component, sysprobeconfig ddconfig.Reader, configPrefix string, log log.Component) error {
	err := settings.SetRuntimeSetting("internal_profiling", false, model.SourceAgentRuntime)
	if err != nil {
		log.Errorf("Error stop profiler: %v", err)
	}

	return err
}

// SetupInternalProfiling is a common helper to configure runtime settings for internal profiling.
func SetupInternalProfiling(settings settings.Component, sysprobeconfig ddconfig.Reader, configPrefix string, log log.Component) {
	if sysprobeconfig.GetBool(configPrefix + "internal_profiling.enabled") {
		startProfiler(settings, sysprobeconfig, configPrefix, log)
	}
}
