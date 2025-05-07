// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a component to run the dogstatsd server
package server

import (
	"encoding/json"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type statsdBlocklistUpdate struct {
	BlockedMetrics blockedMetrics `json:"blocked_metrics"`
}

type blockedMetrics struct {
	ByName byName `json:"by_name"`
}

type byName struct {
	ConfigurationID int           `json:"configuration_id"`
	Metrics         []metricEntry `json:"values"`
}

type metricEntry struct {
	Name string `json:"metric_name"`
}

func (s *server) onBlocklistUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	s.log.Debug("onBlocklistUpdateCallback received updates:", len(updates))

	if len(updates) == 0 {
		return
	}

	var blocklistUpdates []blockedMetrics

	// unmarshal all the configurations received from
	// the RC platform
	// ------------------------

	if len(updates) > 0 {
		// unmarshal all configs that can be unmarshalled
		for configPath, v := range updates {
			s.log.Info("received:", string(v.Config))
			var config statsdBlocklistUpdate
			if err := json.Unmarshal(v.Config, &config); err != nil {
				applyStateCallback(configPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: "error unmarshalling payload",
				})
				s.log.Error("can't unmarshal received config:", err)
				continue
			}
			if len(config.BlockedMetrics.ByName.Metrics) == 0 {
				s.log.Warn("received a configuration with no metric")
				continue
			}
			blocklistUpdates = append(blocklistUpdates, config.BlockedMetrics)
		}
	}

	// sort by the configuration ID
	// ------------------------

	slices.SortFunc(blocklistUpdates, func(a, b blockedMetrics) int {
		return a.ByName.ConfigurationID - b.ByName.ConfigurationID
	})

	// build a map with all the received metrics
	// and then use the values as a blocklist
	// ------------------------

	m := make(map[string]struct{})
	for _, update := range blocklistUpdates {
		for _, metric := range update.ByName.Metrics {
			m[metric.Name] = struct{}{}
		}
	}

	metricsName := make([]string, len(m))
	i := 0
	for metricName := range m {
		metricsName[i] = metricName
		i++
	}

	// apply this blocklist to all the running workers
	// ------------------------

	s.SetBlocklist(metricsName, false)

	// ack the processing to RC
	// ------------------------

	for configPath := range updates {
		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}
}
