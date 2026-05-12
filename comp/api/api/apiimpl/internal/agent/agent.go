// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.
package agent

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/gorilla/mux"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/pkg/api/coverage"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"

	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SetupHandlers adds the specific handlers for /agent endpoints
func SetupHandlers(
	r *mux.Router,
	providers []api.EndpointProvider,
) *mux.Router {
	// Register the handlers from the component providers
	sort.Slice(providers, func(i, j int) bool { return providers[i].Route() < providers[j].Route() })
	for _, p := range providers {
		r.HandleFunc(p.Route(), p.HandlerFunc()).Methods(p.Methods()...)
	}

	// TODO: move these to a component that is registerable
	r.HandleFunc("/status/health", getHealth).Methods("GET")
	r.HandleFunc("/jmx/status", setJMXStatus).Methods("POST")
	r.HandleFunc("/jmx/configs", getJMXConfigs).Methods("GET")
	r.HandleFunc("/install-info", installinfo.HandleGetInstallInfo).Methods("GET")
	r.HandleFunc("/install-info", installinfo.HandleSetInstallInfo).Methods("POST", "PUT")
	coverage.SetupCoverageHandler(r)
	return r
}

func getHealth(w http.ResponseWriter, _ *http.Request) {
	h := health.GetReady()

	if len(h.Unhealthy) > 0 {
		log.Debugf("Healthcheck failed on: %v", h.Unhealthy)
	}

	jsonHealth, err := json.Marshal(h)
	if err != nil {
		log.Errorf("Error marshalling status. Error: %v, Status: %v", err, h)
		httputils.SetJSONError(w, err, 500)
		return
	}

	w.Write(jsonHealth)
}
