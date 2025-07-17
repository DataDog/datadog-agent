// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package version contains helpers to return the agent version in the API
package version

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/version"
)

// Get returns the version of the agent in a http response json
func Get(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	av, _ := version.Agent()
	j, _ := json.Marshal(av)
	w.Write(j)
}
