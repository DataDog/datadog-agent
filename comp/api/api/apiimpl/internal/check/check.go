// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check implements the api endpoints for the `/check` prefix.
// This group of endpoints is meant to provide specific functionalities
// to interact with agent checks.
package check

import (
	"net/http"

	"github.com/gorilla/mux"
)

// SetupHandlers adds the specific handlers for /check endpoints
func SetupHandlers(r *mux.Router) *mux.Router {
	r.HandleFunc("/", listChecks).Methods("GET")
	r.HandleFunc("/{name}", listCheck).Methods("GET", "DELETE")
	r.HandleFunc("/{name}/reload", reloadCheck).Methods("POST")

	return r
}

func reloadCheck(w http.ResponseWriter, r *http.Request) { //nolint:revive // TODO fix revive unused-parameter
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("Not yet implemented."))
}

func listChecks(w http.ResponseWriter, r *http.Request) { //nolint:revive // TODO fix revive unused-parameter
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("Not yet implemented."))
}

func listCheck(w http.ResponseWriter, r *http.Request) { //nolint:revive // TODO fix revive unused-parameter
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("Not yet implemented."))
}
