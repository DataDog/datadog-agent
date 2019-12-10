// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
//
// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.

// +build python
// +build !windows

package agent

import (
	"github.com/segmentio/encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getPythonStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	pyStats, err := python.GetPythonInterpreterMemoryUsage()
	if err != nil {
		log.Warnf("Error getting python stats: %s\n", err) // or something like this
		http.Error(w, err.Error(), 500)
	}

	j, _ := json.Marshal(pyStats)
	w.Write(j)
}
