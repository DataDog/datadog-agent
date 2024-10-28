// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package api

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func checkHandler(w http.ResponseWriter, req *http.Request) {
	requestedCheck := mux.Vars(req)["check"]
	checkOutput, ok := checks.GetCheckOutput(requestedCheck)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, err := io.WriteString(w, fmt.Sprintf("%s check is not running or has not been scheduled yet\n", html.EscapeString(requestedCheck)))
		if err != nil {
			_ = log.Error()
		}
		return
	}

	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	err := e.Encode(checkOutput)
	if err != nil {
		writeError(err, http.StatusInternalServerError, w)
		_ = log.Error(err)
		return
	}
}
