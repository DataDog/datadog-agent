// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a component to run the dogstatsd server
package server

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type statsdFilterListUpdate struct {
	FilteredMetrics filteredMetrics `json:"blocked_metrics"`
}

type filteredMetrics struct {
	ByName byName `json:"by_name"`
}

type byName struct {
	Metrics []metricEntry `json:"values"`
}

type metricEntry struct {
	Name string `json:"metric_name"`
}

func (s *server) onFilterListUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	s.log.Debugf("onFilterListUpdateCallback received updates: %d", len(updates))

	// special case: we received a response from RC, but RC didn't have any
	// configuration for this agent, let's restore the local config and return
	if len(updates) == 0 {
		s.config.UnsetForSource("statsd_metric_blocklist", model.SourceRC)
		s.config.UnsetForSource("statsd_metric_blocklist_match_prefix", model.SourceRC)
		s.restoreFilterListFromLocalConfig()
		return
	}

	var filterListUpdates []filteredMetrics

	// unmarshal all the configurations received from
	// the RC platform
	for configPath, v := range updates {
		s.log.Debugf("received filterlist config: %q", string(v.Config))
		var config statsdFilterListUpdate
		if err := json.Unmarshal(v.Config, &config); err != nil {
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "error unmarshalling payload",
			})
			s.log.Errorf("can't unmarshal received filterlist config: %v", err)
			continue
		}

		// from here, the configuration is usable
		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})

		// this one has no metric in its list, strange but
		// not an error
		if len(config.FilteredMetrics.ByName.Metrics) == 0 {
			s.log.Debug("received a filterlist configuration with no metrics")
			continue
		}
		filterListUpdates = append(filterListUpdates, config.FilteredMetrics)
	}

	// build a map with all the received metrics
	// and then use the values as a filterlist
	m := make(map[string]struct{})
	for _, update := range filterListUpdates {
		for _, metric := range update.ByName.Metrics {
			m[metric.Name] = struct{}{}
		}
	}
	metricNames := slices.Collect(maps.Keys(m))

	if len(metricNames) > 0 {
		// update the runtime config to be consistent
		// in `agent config` calls.
		s.config.Set("statsd_metric_blocklist", metricNames, model.SourceRC)
		s.config.Set("statsd_metric_blocklist_match_prefix", false, model.SourceRC)

		// apply this new blocklist to all the running workers
		s.tlmFilterListUpdates.Inc()
		s.tlmFilterListSize.Set(float64(len(metricNames)))
		s.SetFilterList(metricNames, false)

	} else {
		// special case: if the metric names list is empty, fallback to local
		s.config.UnsetForSource("statsd_metric_blocklist", model.SourceRC)
		s.config.UnsetForSource("statsd_metric_blocklist_match_prefix", model.SourceRC)
		s.restoreFilterListFromLocalConfig()
	}
}
