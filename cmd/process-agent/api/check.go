// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

func checkHandler(w http.ResponseWriter, req *http.Request) {
	requestedCheck := mux.Vars(req)["check"]
	for _, check := range checks.All {
		if check.Name() == requestedCheck {
			payload, err := check.Run(nil, 0)
			if err != nil {
				writeError(err, http.StatusInternalServerError, w)
				_ = log.Error(err)
				return
			}

			b, err := json.Marshal(&payload)
			if err != nil {
				writeError(err, http.StatusInternalServerError, w)
				_ = log.Error(err)
				return
			}

			_, err = w.Write(b)
			if err != nil {
				_ = log.Error(err)
			}
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}
