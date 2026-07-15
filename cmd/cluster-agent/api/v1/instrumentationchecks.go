// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func installInstrumentationCheckEndpoints(r *http.ServeMux, confLister clusteragent.ConfigLister) {
	r.HandleFunc("GET /instrumentation/configs", api.WithTelemetryWrapper("getInstrumentationConfigs", getInstrumentationConfigs(confLister)))
	r.HandleFunc("GET /instrumentation/status", api.WithTelemetryWrapper("getInstrumentationStatus", getInstrumentationStatus(confLister)))
}

func getInstrumentationConfigs(confLister clusteragent.ConfigLister) func(w http.ResponseWriter, r *http.Request) {
	if confLister == nil {
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "instrumentation config provider not available", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, _ *http.Request) {
		configs, hash := confLister.ListConfigs()
		response := cctypes.InstrumentationConfigResponse{
			ConfigHash: hash,
			Configs:    configs,
		}
		slcB, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(slcB) //nolint:errcheck
	}
}

func getInstrumentationStatus(confLister clusteragent.ConfigLister) func(w http.ResponseWriter, r *http.Request) {
	if confLister == nil {
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "instrumentation config provider not available", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, _ *http.Request) {
		response := cctypes.InstrumentationStatusResponse{
			ConfigHash: confLister.Hash(),
		}
		slcB, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(slcB) //nolint:errcheck
	}
}
