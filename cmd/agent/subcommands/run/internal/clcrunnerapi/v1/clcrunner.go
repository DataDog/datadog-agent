// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package v1 implements the api endpoints for the `/api/v1` prefix.
// This group of endpoints is meant to provide external queries with
// stats of the agent.
package v1

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// IMPORTANT NOTE:
// Every payload change requires a version bump of the API
// This API is NOT meant to:
// - expose check configs
// - configure the Agent or change its behaviour

// SetupHandlers adds the specific handlers for /api/v1 endpoints
// The API is only meant to expose stats used by the Cluster Agent
// Check configs and any data that could contain sensitive information
// MUST NEVER be sent via this API
func SetupHandlers(r *mux.Router, ac autodiscovery.Component) {
	r.HandleFunc("/clcrunner/version", common.GetVersion).Methods("GET")
	r.HandleFunc("/clcrunner/stats", func(w http.ResponseWriter, r *http.Request) {
		getCLCRunnerStats(w, r, ac)
	}).Methods("GET")
	r.HandleFunc("/clcrunner/workers", getCLCRunnerWorkers).Methods("GET")
}

// getCLCRunnerStats retrieves Cluster Level Check runners stats
func getCLCRunnerStats(w http.ResponseWriter, _ *http.Request, ac autodiscovery.Component) {
	log.Info("Got a request for the runner stats. Making stats.")
	w.Header().Set("Content-Type", "application/json")
	stats, err := status.GetExpvarRunnerStats()
	if err != nil {
		log.Errorf("Error getting exp var stats: %v", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}
	flattenedStats := flattenCLCStats(stats)
	statsWithIDsKnownByDCA := replaceIDsWithIDsKnownByDCA(ac, flattenedStats)
	jsonStats, err := json.Marshal(statsWithIDsKnownByDCA)
	if err != nil {
		log.Errorf("Error marshalling stats. Error: %v, Stats: %v", err, statsWithIDsKnownByDCA)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(jsonStats)
}

// flattenCLCStats simplifies the status.CLCChecks struct by making it a map
func flattenCLCStats(stats status.CLCChecks) map[string]status.CLCStats {
	flatened := make(map[string]status.CLCStats)
	for _, checks := range stats.Checks {
		for checkID, checkStats := range checks {
			flatened[checkID] = checkStats
		}
	}

	return flatened
}

// replaceIDsWithIDsKnownByDCA replaces the check IDs in the map received with
// the ID that those checks had before decrypting their secrets. This is needed
// because if the Cluster Agent does not decrypt secrets and the runner does,
// the check ID seen by both of them is going to be different and the Cluster
// Agent won't recognize the check as a cluster check.
// The API defined in this file is only used by the Cluster Agent, so it makes
// sense to use the IDs that it recognizes.
func replaceIDsWithIDsKnownByDCA(ac autodiscovery.Component, stats map[string]status.CLCStats) map[string]status.CLCStats {
	res := make(map[string]status.CLCStats, len(stats))

	for checkID, checkStats := range stats {
		originalID := ac.GetIDOfCheckWithEncryptedSecrets(checkid.ID(checkID))

		if originalID != "" {
			res[string(originalID)] = checkStats
		} else {
			res[checkID] = checkStats
		}
	}

	return res
}

func getCLCRunnerWorkers(w http.ResponseWriter, _ *http.Request) {
	log.Info("Got a request for the runner workers")
	w.Header().Set("Content-Type", "application/json")
	stats, err := status.GetExpvarRunnerStats()
	if err != nil {
		log.Errorf("Error getting exp var stats: %v", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	jsonWorkers, err := json.Marshal(stats.Workers)
	if err != nil {
		log.Errorf("Error marshalling stats. Error: %v, Stats: %v", err, stats.Workers)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(jsonWorkers)
}
