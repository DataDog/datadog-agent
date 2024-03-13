// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"
)

func statusHandler(deps APIServerDeps, w http.ResponseWriter, _ *http.Request) {
	deps.Log.Info("Got a request for the status. Making status.")

	bytes, err := deps.Status.GetStatus("text", false)
	if err != nil {
		_ = deps.Log.Warn("failed to get status response from agent:", err)
		writeError(err, http.StatusInternalServerError, w)
		return
	}
	_, err = w.Write(bytes)
	if err != nil {
		_ = deps.Log.Warn("received response from agent but failed write it to client:", err)
	}
}
