// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.

//go:build e2ecoverage

// Package coverage implements the api endpoints for the `/coverage` prefix.
package coverage

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"runtime/coverage"

	"github.com/gorilla/mux"
)

// SetupCoverageHandler adds the coverage handler to the router
func SetupCoverageHandler(r *mux.Router) {
	r.HandleFunc("/coverage", ComponentCoverageHandler).Methods("GET")
}

func ComponentCoverageHandler(w http.ResponseWriter, _ *http.Request) {
	tempDir := path.Join(os.TempDir(), "coverage")
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = coverage.WriteCountersDir(tempDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = coverage.WriteMetaDir(tempDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(fmt.Sprintf("Coverage written to %s", tempDir)))
}
