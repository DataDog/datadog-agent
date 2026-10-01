// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"encoding/json"
	"slices"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/twmb/murmur3"
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
	// configuration for this agent, let's restore the local config and return
	if len(updates) == 0 {
		fl.config.UnsetForSource("metric_filterlist", model.SourceRC)
		fl.config.UnsetForSource("metric_filterlist_match_prefix", model.SourceRC)
		fl.config.UnsetForSource("metric_filterlist_prefix", model.SourceRC)
		fl.config.UnsetForSource("statsd_metric_blocklist", model.SourceRC)
		fl.config.UnsetForSource("statsd_metric_blocklist_match_prefix", model.SourceRC)
		fl.config.UnsetForSource("metric_tag_filterlist", model.SourceRC)
		fl.restoreMetricFilterListFromLocalConfig()
		fl.restoreTagFilterListFromLocalConfig()
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
	effectiveNames := fl.localFilterListConfig.metricNames
	effectiveMatchPrefix := fl.localFilterListConfig.matchPrefix

	metricNames := fl.buildMetricFilterListConfig(metricFilterListUpdates)
	// RC metric names are exact; per-entry prefixes use metric_filterlist_prefix.
	metricNames = normalizeMetricNames(metricNames, false, fl.log)

	if len(metricNames) > 0 {
		// update the runtime config to be consistent
		// in `agent config` calls.
		fl.config.Set("metric_filterlist", metricNames, model.SourceRC)
		fl.config.Set("metric_filterlist_match_prefix", false, model.SourceRC)
		if len(fl.localFilterListConfig.metricNames) > 0 {
			fl.config.Set("statsd_metric_blocklist", []string{}, model.SourceRC)
			fl.config.Set("statsd_metric_blocklist_match_prefix", false, model.SourceRC)
		}

		effectiveNames = metricNames
		effectiveMatchPrefix = false
	} else {
		fl.config.UnsetForSource("metric_filterlist", model.SourceRC)
		fl.config.UnsetForSource("metric_filterlist_match_prefix", model.SourceRC)
		fl.config.UnsetForSource("statsd_metric_blocklist", model.SourceRC)
		fl.config.UnsetForSource("statsd_metric_blocklist_match_prefix", model.SourceRC)
	}

	effectivePrefixRules := fl.localFilterListConfig.metricPrefixRules

	prefixEntries := fl.buildMetricPrefixListConfig(metricPrefixListUpdates)
	if len(prefixEntries) > 0 {
		fl.config.Set("metric_filterlist_prefix", prefixEntries, model.SourceRC)

		effectivePrefixRules = normalizeMetricPrefixList(prefixEntries, fl.log)
	} else {
		fl.config.UnsetForSource("metric_filterlist_prefix", model.SourceRC)
	}

	fl.SetMetricFilterList(effectiveNames, effectiveMatchPrefix, effectivePrefixRules)

	tags, tagEntries := fl.buildTagFilterListConfig(tagFilterListUpdates)

	if len(tags) > 0 {
		// Convert map to slice for config storage
		// update the runtime config to be consistent
		// in `agent config` calls.
		fl.config.Set("metric_tag_filterlist", tagEntries, model.SourceRC)

		// apply this new blocklist to all the running workers
		fl.setTagFilterList(tagMatcher{
			MetricTags: tags,
		})
	} else {
		// special case: if the metric names list is empty, fallback to local
		fl.config.UnsetForSource("metric_tag_filterlist", model.SourceRC)
		fl.restoreTagFilterListFromLocalConfig()
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
	metricNames := make([]string, 0, len(metrics))
	for metric := range metrics {
		metricNames = append(metricNames, metric)
	}
	sort.Strings(metricNames)
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

// buildConfig builds the configuration to use.
// The first result contains the hashed tags that the filterlist uses, second result is the entry with unhashed tags
// used to override the configuration file entry.
//
// There is nothing stopping the tags for a given metric name being configured multiple times. Conflicts are handled
// by the following rules:
// - If the action is the same for both metrics, the list of tags is merged.
// - If the action is different, always take the exclude list.
func (fl *FilterList) buildTagFilterListConfig(tagFilterListUpdates []filteredTags) (map[string]hashedMetricTagList, []MetricTagListEntry) {
	tags := make(map[string]hashedMetricTagList)
	tagEntries := make(map[string]MetricTagListEntry)

	for _, update := range tagFilterListUpdates {
		for _, metric := range update.ByName.Metrics {
			currentHashed, ok := tags[metric.Name]
			currentEntry := tagEntries[metric.Name]

			actionStr := "include"
			if metric.ExcludeTag {
				actionStr = "exclude"
			}

			if ok {
				// Metric has already been defined, merge it.
				hashed, entry := fl.mergeMetricTagListEntry(metric, currentHashed, currentEntry)
				tags[metric.Name] = hashed
				tagEntries[metric.Name] = entry
			} else {
				hashedTags := hashTags(metric.Tags)
				var rcAction action
				if metric.ExcludeTag {
					rcAction = exclude
				} else {
					rcAction = include
				}
				tags[metric.Name] = newHashedMetricTagList(rcAction, hashedTags)

				// Store unhashed entry
				tagEntries[metric.Name] = MetricTagListEntry{
					MetricName: metric.Name,
					Action:     actionStr,
					Tags:       metric.Tags,
				}
			}
		}
	}

	tagEntriesSlice := make([]MetricTagListEntry, 0, len(tagEntries))
	for _, entry := range tagEntries {
		tagEntriesSlice = append(tagEntriesSlice, entry)
	}

	return tags, tagEntriesSlice
}

// mergeMetricTagListEntry merges the given metric entry with the current entry.
// It needs to merge with both the hashed and unhashed variants.
func (fl *FilterList) mergeMetricTagListEntry(metric tagEntry, currentHashed hashedMetricTagList, currentEntry MetricTagListEntry) (hashedMetricTagList, MetricTagListEntry) {
	if (currentHashed.action == exclude) == metric.ExcludeTag {
		// Both metrics define the same action so we can just merge the list.
		currentHashed.tags = append(currentHashed.tags, hashTags(metric.Tags)...)
		slices.Sort(currentHashed.tags)

		// Merge unhashed tags too
		currentEntry.Tags = append(currentEntry.Tags, metric.Tags...)
		return currentHashed, currentEntry
	} else if currentHashed.action == include {
		// We always prefer the exclude tag, overwrite the existing config with this one.
		hashedTags := hashTags(metric.Tags)

		// Overwrite unhashed entry with exclude
		hashed := newHashedMetricTagList(exclude, hashedTags)

		entry := MetricTagListEntry{
			MetricName: metric.Name,
			Action:     "exclude",
			Tags:       metric.Tags,
		}
		fl.log.Debugf("tag filterlist configures conflicting tags for metric %v", metric.Name)

		return hashed, entry
	}

	// We always prefer the exclude tag, ignore this include tag configuration.
	fl.log.Debugf("tag filterlist configures conflicting tags for metric %v", metric.Name)
	return currentHashed, currentEntry
}

func hashTags(tags []string) []uint64 {
	hashed := make([]uint64, 0, len(tags))
	for _, tag := range tags {
		hashed = append(hashed, murmur3.StringSum64(tag))
	}

	return hashed
}
