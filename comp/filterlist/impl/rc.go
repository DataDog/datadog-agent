// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/twmb/murmur3"
)

type statsdFilterListUpdate struct {
	FilteredMetrics filteredMetrics `json:"blocked_metrics"`
	FilteredTags    filteredTags    `json:"tag_filterlist"`
}

type filteredTags struct {
	ByName tagByName `json:"by_name"`
}

type tagByName struct {
	Metrics []tagEntry `json:"by_name"`
}

type tagEntry struct {
	Name       string   `json:"metric_name"`
	ExcludeTag bool     `json:"exclude_tag_mode"`
	Tags       []string `json:"tags"`
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

func (fl *FilterList) onFilterListUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	fl.log.Debugf("onFilterListUpdateCallback received updates: %d", len(updates))

	// special case: we received a response from RC, but RC didn't have any
	// configuration for this agent, let's restore the local config and return
	if len(updates) == 0 {
		fl.config.UnsetForSource("metric_filterlist", model.SourceRC)
		fl.config.UnsetForSource("metric_filterlist_match_prefix", model.SourceRC)
		fl.config.UnsetForSource("statsd_metric_blocklist", model.SourceRC)
		fl.config.UnsetForSource("statsd_metric_blocklist_match_prefix", model.SourceRC)
		fl.restoreFilterListFromLocalConfig()
		return
	}

	var filterListUpdates []filteredMetrics

	// unmarshal all the configurations received from
	// the RC platform
	for configPath, v := range updates {
		fl.log.Debugf("received filterlist config: %q", string(v.Config))
		var config statsdFilterListUpdate
		if err := json.Unmarshal(v.Config, &config); err != nil {
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "error unmarshalling payload",
			})
			fl.log.Errorf("can't unmarshal received filterlist config: %v", err)
			continue
		}

		// from here, the configuration is usable
		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})

		// this one has no metric in its list, strange but
		// not an error
		if len(config.FilteredMetrics.ByName.Metrics) == 0 {
			fl.log.Debug("received a filterlist configuration with no metrics")
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
		fl.config.Set("metric_filterlist", metricNames, model.SourceRC)
		fl.config.Set("metric_filterlist_match_prefix", false, model.SourceRC)
		if len(fl.localFilterListConfig.metricNames) > 0 {
			fl.config.Set("statsd_metric_blocklist", []string{}, model.SourceRC)
			fl.config.Set("statsd_metric_blocklist_match_prefix", false, model.SourceRC)
		}

		// apply this new blocklist to all the running workers
		fl.tlmFilterListUpdates.Inc()
		fl.tlmFilterListSize.Set(float64(len(metricNames)))
		fl.SetFilterList(metricNames, false)
	} else {
		// special case: if the metric names list is empty, fallback to local
		fl.config.UnsetForSource("metric_filterlist", model.SourceRC)
		fl.config.UnsetForSource("metric_filterlist_match_prefix", model.SourceRC)
		fl.config.UnsetForSource("statsd_metric_blocklist", model.SourceRC)
		fl.config.UnsetForSource("statsd_metric_blocklist_match_prefix", model.SourceRC)
		fl.restoreFilterListFromLocalConfig()
	}
}

func (fl *FilterList) onTagFilterListUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	fl.log.Debugf("onFilterListUpdateCallback received updates: %d", len(updates))

	// special case: we received a response from RC, but RC didn't have any
	// configuration for this agent, let's restore the local config and return
	if len(updates) == 0 {
		fl.config.UnsetForSource("metric_tag_filterlist", model.SourceRC)
		// TODO restore tag filterlist.
		//fl.restoreFilterListFromLocalConfig()
		return
	}

	var tagFilterListUpdates []filteredTags

	// unmarshal all the configurations received from
	// the RC platform
	for configPath, v := range updates {
		fl.log.Debugf("received tag filterlist config: %q", string(v.Config))
		var config statsdFilterListUpdate
		if err := json.Unmarshal(v.Config, &config); err != nil {
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "error unmarshalling payload",
			})
			fl.log.Errorf("can't unmarshal received tag filterlist config: %v", err)
			continue
		}

		// from here, the configuration is usable
		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})

		// this one has no metric in its list, strange but
		// not an error
		if len(config.FilteredTags.ByName.Metrics) == 0 {
			fl.log.Debug("received a tag filterlist configuration with no metrics")
			continue
		}
		tagFilterListUpdates = append(tagFilterListUpdates, config.FilteredTags)
	}

	// build a map with all the received metrics
	// and then use the values as a filterlist
	m := make(map[string]hashedMetricTagList)
	for _, update := range tagFilterListUpdates {
		for _, metric := range update.ByName.Metrics {
			current, ok := m[metric.Name]
			if ok {
				// Metric has already been defined, merge it.
				if (current.action == Exclude) == metric.ExcludeTag {
					// Both metrics define the same action so we can just merge the list.
					for _, tag := range metric.Tags {
						current.tags = append(current.tags, murmur3.StringSum64(tag))
					}
				} else if current.action == Include {
					// We always prefer the exclude tag, overwrite the existing config with this one.
					current.action = Exclude
					tags := make([]uint64, 0, len(metric.Tags))
					for _, tag := range metric.Tags {
						tags = append(tags, murmur3.StringSum64(tag))
					}
					current.tags = tags
					fl.log.Debug("tag filterlist configures conflicting tags for metric %v", metric.Name)
				} else {
					// We always prefer the exclude tag, ignore this include tag configuration.
					fl.log.Debug("tag filterlist configures conflicting tags for metric %v", metric.Name)
				}
			} else {
				tags := make([]uint64, 0, len(metric.Tags))
				for _, tag := range metric.Tags {
					tags = append(tags, murmur3.StringSum64(tag))
				}
				var action action
				if metric.ExcludeTag {
					action = Exclude
				} else {
					action = Include
				}
				m[metric.Name] = hashedMetricTagList{
					action: action,
					tags:   tags,
				}
			}
		}
	}

	if len(m) > 0 {
		// update the runtime config to be consistent
		// in `agent config` calls.
		// fl.config.Set("metric_tag_filterlist", metricNames, model.SourceRC)

		// apply this new blocklist to all the running workers
		fl.tlmFilterListUpdates.Inc()
		fl.tlmFilterListSize.Set(float64(len(m)))
		fl.SetTagFilterList(m)
	} else {
		// special case: if the metric names list is empty, fallback to local
		fl.config.UnsetForSource("metric_tag_filterlist", model.SourceRC)
		// TODO update local
		// fl.restoreFilterListFromLocalConfig()
	}
}
