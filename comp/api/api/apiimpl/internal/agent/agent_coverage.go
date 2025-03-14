// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2ecoverage

package agent

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"runtime/coverage"

	"github.com/gorilla/mux"
)

func setupCoverageHandler(r *mux.Router) {
	r.HandleFunc("/coverage", func(w http.ResponseWriter, r *http.Request) {
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
	}).Methods("GET")
}
