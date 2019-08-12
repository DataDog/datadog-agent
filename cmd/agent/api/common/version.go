// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package common implements shared functions between the Agent APIs.
package common

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/version"
)

// GetVersion returns the version of the agent
func GetVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	av, _ := version.New(version.AgentVersion, version.Commit)
	j, _ := json.Marshal(av)
	w.Write(j)
}
