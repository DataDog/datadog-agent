// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func installInstrumentationCheckEndpoints(r *http.ServeMux, confLister clusteragent.ConfigLister, statusReceiver clusteragent.InstrumentationCheckStatusReceiver) {
	r.HandleFunc("GET /instrumentation/configs", api.WithTelemetryWrapper("getInstrumentationConfigs", getInstrumentationConfigs(confLister)))
	r.HandleFunc("GET /instrumentation/status", api.WithTelemetryWrapper("getInstrumentationStatus", getInstrumentationStatus(confLister)))
	r.HandleFunc("POST /instrumentation/check-status", api.WithTelemetryWrapper("postInstrumentationCheckStatus", postInstrumentationCheckStatus(statusReceiver)))
}

func postInstrumentationCheckStatus(receiver clusteragent.InstrumentationCheckStatusReceiver) func(w http.ResponseWriter, r *http.Request) {
	if receiver == nil {
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "instrumentation check status receiver not available", http.StatusServiceUnavailable)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var request cctypes.InstrumentationCheckStatusRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		receiver.SubmitInstrumentationCheckStatus(request)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}
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
