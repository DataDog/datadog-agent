// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package dogstatsd

import (
	"github.com/gorilla/mux"

	statusapi "github.com/DataDog/datadog-agent/pkg/status/api"
	taggerapi "github.com/DataDog/datadog-agent/pkg/tagger/api"
)

// SetupHandlers adds the specific handlers for /agent endpoints
func SetupHandlers(r *mux.Router) {
	// From pkg/tagger/api
	r.HandleFunc("/tagger-list", taggerapi.ListHandler).Methods("GET")
	// From pkg/status/api
	r.HandleFunc("/status/health", statusapi.HealthHandler).Methods("GET")
}
