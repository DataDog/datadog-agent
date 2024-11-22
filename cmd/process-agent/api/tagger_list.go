// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"encoding/json"
	"net/http"

	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

//nolint:revive // TODO(PROC) Fix revive linter
func getTaggerList(deps APIServerDeps, w http.ResponseWriter, _ *http.Request) {
	response := deps.Tagger.List()

	jsonTags, err := json.Marshal(response)
	if err != nil {
		httputils.SetJSONError(w, deps.Log.Errorf("Unable to marshal tagger list response: %s", err), 500)
		return
	}
	w.Write(jsonTags)
}
