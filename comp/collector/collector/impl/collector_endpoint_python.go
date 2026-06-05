// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build python && !windows

// Package collectorimpl provides the implementation of the collector component.
package collectorimpl

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getPythonStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	pyStats, err := python.GetPythonInterpreterMemoryUsage()
	if err != nil {
		log.Warnf("Error getting python stats: %s\n", err) // or something like this
		http.Error(w, err.Error(), 500)
	}

	j, _ := json.Marshal(pyStats)
	w.Write(j)
}
