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
)

type statsdFilterListUpdate struct {
	FilteredMetrics        filteredMetrics        `json:"blocked_metrics"`
	FilteredMetricPrefixes filteredMetricPrefixes `json:"metric_filterlist_prefix"`
	FilteredTags           filteredTags           `json:"tag_filterlist"`
}

type filteredMetricPrefixes struct {
	ByPrefix byPrefix `json:"by_prefix"`
}

type byPrefix struct {
	Values []prefixEntry `json:"values"`
}

// RC encodes exceptions as objects, unlike local YAML.
type prefixEntry struct {
	Prefix       string             `json:"prefix"`
	ExceptPrefix []exceptPrefixName `json:"except_prefix"`
	ExceptExact  []exceptExactName  `json:"except_exact"`
}

type exceptPrefixName struct {
	Prefix string `json:"prefix"`
}

type exceptExactName struct {
	Name string `json:"name"`
}

type filteredTags struct {
	ByName tagByName `json:"by_name"`
}

type tagByName struct {
	Metrics []tagEntry `json:"values"`
}

type tagEntry struct {
	Name       string   `json:"metric_name"`
	ExcludeTag bool     `json:"exclude_tags_mode"`
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

// onFilterListUpdateCallback receives both metric and tag filterlist configurations.
func (fl *FilterList) onFilterListUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	fl.log.Debugf("onFilterListUpdateCallback received updates: %d", len(updates))

	// special case: we received a response from RC, but RC didn't have any
	// configuration for this agent, so clear the RC layer and restore from config.
	if len(updates) == 0 {
		fl.config.UnsetForSource("metric_filterlist", model.SourceRC)
		fl.config.UnsetForSource("metric_filterlist_match_prefix", model.SourceRC)
		fl.config.UnsetForSource("metric_filterlist_prefix", model.SourceRC)
		fl.config.UnsetForSource("metric_tag_filterlist", model.SourceRC)
		fl.restoreMetricFilterListFromConfig()
		fl.restoreTagFilterListFromConfig()
		return
	}

	var metricFilterListUpdates []filteredMetrics
	var metricPrefixListUpdates []filteredMetricPrefixes
	var tagFilterListUpdates []filteredTags

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

		// Empty configs are unusual, but valid.
		if len(config.FilteredMetrics.ByName.Metrics) == 0 &&
			len(config.FilteredMetricPrefixes.ByPrefix.Values) == 0 &&
			len(config.FilteredTags.ByName.Metrics) == 0 {

			fl.log.Debug("received a filterlist configuration with no metrics")
			continue
		}
		metricFilterListUpdates = append(metricFilterListUpdates, config.FilteredMetrics)
		metricPrefixListUpdates = append(metricPrefixListUpdates, config.FilteredMetricPrefixes)
		tagFilterListUpdates = append(tagFilterListUpdates, config.FilteredTags)
	}

	// Exact and prefix-rule lists are separate RC fields; resolve them independently.
	metricNames := fl.buildMetricFilterListConfig(metricFilterListUpdates)
	// RC metric names are exact; per-entry prefixes use metric_filterlist_prefix.
	metricNames = normalizeMetricNames(metricNames, false, fl.log)

	if len(metricNames) > 0 {
		// update the runtime config to be consistent
		// in `agent config` calls.
		fl.config.Set("metric_filterlist", metricNames, model.SourceRC)
		fl.config.Set("metric_filterlist_match_prefix", false, model.SourceRC)
	} else {
		fl.config.UnsetForSource("metric_filterlist", model.SourceRC)
		fl.config.UnsetForSource("metric_filterlist_match_prefix", model.SourceRC)
	}

	prefixEntries := fl.buildMetricPrefixListConfig(metricPrefixListUpdates)
	if len(prefixEntries) > 0 {
		fl.config.Set("metric_filterlist_prefix", prefixEntries, model.SourceRC)
	} else {
		fl.config.UnsetForSource("metric_filterlist_prefix", model.SourceRC)
	}

	fl.restoreMetricFilterListFromConfig()

	tagEntries := fl.buildTagFilterListConfig(tagFilterListUpdates)

	if len(tagEntries) > 0 {
		// Update the runtime config to be consistent in `agent config`
		// calls. Entries are reported as received; normalization only
		// happens when compiling the matcher.
		fl.config.Set("metric_tag_filterlist", tagEntries, model.SourceRC)

		// apply this new blocklist to all the running workers
		fl.setTagFilterList(compileTagFilterList(tagEntries, fl.log))
	} else {
		fl.config.UnsetForSource("metric_tag_filterlist", model.SourceRC)
		fl.restoreTagFilterListFromConfig()
	}
}

// buildMetricFilterListConfig builds exact-name metric_filterlist entries.
func (*FilterList) buildMetricFilterListConfig(metricFilterListUpdates []filteredMetrics) []string {
	metrics := make(map[string]struct{})
	for _, update := range metricFilterListUpdates {
		for _, metric := range update.ByName.Metrics {
			metrics[metric.Name] = struct{}{}
		}
	}
	metricNames := slices.Collect(maps.Keys(metrics))
	return metricNames
}

// buildMetricPrefixListConfig converts RC prefix rules into the local YAML shape.
// Duplicate prefixes are preserved; prefix exceptions are applied globally.
func (*FilterList) buildMetricPrefixListConfig(metricPrefixListUpdates []filteredMetricPrefixes) []MetricPrefixListEntry {
	var prefixEntries []MetricPrefixListEntry
	for _, update := range metricPrefixListUpdates {
		for _, v := range update.ByPrefix.Values {
			entry := MetricPrefixListEntry{Prefix: v.Prefix}
			for _, exceptPrefix := range v.ExceptPrefix {
				entry.ExceptPrefix = append(entry.ExceptPrefix, exceptPrefix.Prefix)
			}
			for _, exceptExact := range v.ExceptExact {
				entry.ExceptExact = append(entry.ExceptExact, exceptExact.Name)
			}
			prefixEntries = append(prefixEntries, entry)
		}
	}
	return prefixEntries
}

// buildTagFilterListConfig builds the metric_tag_filterlist entries received
// from RC, in the shape of the local configuration. The matcher is compiled
// from them with compileTagFilterList, exactly as for the local configuration.
//
// There is nothing stopping the tags for a given metric name being configured multiple times. Conflicts are handled
// by the following rules:
// - If the action is the same for both metrics, the list of tags is merged.
// - If the action is different, always take the exclude list.
func (fl *FilterList) buildTagFilterListConfig(tagFilterListUpdates []filteredTags) []MetricTagListEntry {
	tagEntries := make(map[string]MetricTagListEntry)

	for _, update := range tagFilterListUpdates {
		for _, metric := range update.ByName.Metrics {
			if currentEntry, ok := tagEntries[metric.Name]; ok {
				// Metric has already been defined, merge it.
				tagEntries[metric.Name] = fl.mergeMetricTagListEntry(metric, currentEntry)
				continue
			}

			actionStr := "include"
			if metric.ExcludeTag {
				actionStr = "exclude"
			}
			tagEntries[metric.Name] = MetricTagListEntry{
				MetricName: metric.Name,
				Action:     actionStr,
				Tags:       metric.Tags,
			}
		}
	}

	tagEntriesSlice := make([]MetricTagListEntry, 0, len(tagEntries))
	for _, entry := range tagEntries {
		tagEntriesSlice = append(tagEntriesSlice, entry)
	}

	return tagEntriesSlice
}

// mergeMetricTagListEntry merges the given metric entry with the current entry.
func (fl *FilterList) mergeMetricTagListEntry(metric tagEntry, currentEntry MetricTagListEntry) MetricTagListEntry {
	currentIsExclude := currentEntry.Action == "exclude"
	if currentIsExclude == metric.ExcludeTag {
		// Both metrics define the same action so we can just merge the list.
		currentEntry.Tags = append(slices.Clip(currentEntry.Tags), metric.Tags...)
		return currentEntry
	}

	fl.log.Debugf("tag filterlist configures conflicting tags for metric %v", metric.Name)
	if !currentIsExclude {
		// We always prefer the exclude tag, overwrite the existing config with this one.
		return MetricTagListEntry{
			MetricName: metric.Name,
			Action:     "exclude",
			Tags:       metric.Tags,
		}
	}

	// We always prefer the exclude tag, ignore this include tag configuration.
	return currentEntry
}
