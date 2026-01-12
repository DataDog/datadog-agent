// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"
)

func secretRefreshHandler(deps APIServerDeps, w http.ResponseWriter, _ *http.Request) {
	response, err := deps.Secrets.Refresh(true)
	if err != nil {
		deps.Log.Errorf("error while refreshing secrets: %s", err)
		writeError(err, http.StatusInternalServerError, w)
		return
	}
	w.Write([]byte(response))
}
