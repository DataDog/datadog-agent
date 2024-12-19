// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/api/util"
)

func configSetHandler(deps APIServerDeps, w http.ResponseWriter, r *http.Request) {
	if err := util.Validate(w, r); err != nil {
		deps.Log.Warnf("invalid auth token for %s request to %s: %s", r.Method, r.RequestURI, err)
		return
	}

	deps.Settings.SetValue(w, r)
}
