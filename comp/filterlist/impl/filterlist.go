// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterlistimpl provides the implementation for the filterlist/rc component
package filterlistimpl

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	filterlistdef "github.com/DataDog/datadog-agent/comp/filterlist/def"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
)

// Requires contains the config for RC
type Requires struct {
	Cfg       config.Component
	Log       log.Component
	Telemetry telemetry.Component
}

// Provides contains the RC component
type Provides struct {
	Comp       filterlistdef.Component
	RCListener rctypes.ListenerProvider
}

type localFilterListConfig struct {
	metricNames   []string
	matchPrefix   bool
	tagFilterList []MetricTagListEntry
}

type FilterList struct {
	localFilterListConfig

	log           log.Component
	config        config.Component
	telemetryComp telemetry.Component

	updateMetricMtx        sync.RWMutex
	metricFilterListUpdate []func(metricname.Matcher, metricname.Matcher)
	filterList             metricname.Matcher
	histoFilterList        metricname.Matcher

	updateTagMtx        sync.RWMutex
	tagFilterListUpdate []func(filterlistdef.TagMatcher)
	tagFilterList       tagMatcher

	tlmMetricFilterListUpdates telemetry.SimpleCounter
	tlmMetricFilterListSize    telemetry.SimpleGauge

	tlmTagFilterListUpdates telemetry.SimpleCounter
	tlmTagFilterListSize    telemetry.SimpleGauge
}

// NewFilterList loads the local config.
// Note that registering with RC happens via separate methods
// (OnUpdateMetricFilterList, OnUpdateTagFilterList) called from
// the packages that use FilterList.
func NewFilterList(log log.Component, config config.Component, telemetryComp telemetry.Component) *FilterList {
	// init the metric names filterlist
	filterlist := config.GetStringSlice("metric_filterlist")
	filterlistPrefix := config.GetBool("metric_filterlist_match_prefix")
	if len(filterlist) == 0 {
		filterlist = config.GetStringSlice("statsd_metric_blocklist")
		filterlistPrefix = config.GetBool("statsd_metric_blocklist_match_prefix")
	}
	filterlist = normalizeMetricNames(filterlist, filterlistPrefix, log)

	// Load tag filter list from config
	var tagFilterListEntries []MetricTagListEntry
	err := structure.UnmarshalKey(config, "metric_tag_filterlist", &tagFilterListEntries)
	if err != nil {
		log.Errorf("error loading metric_tag_filterlist configuration: %s", err)
		tagFilterListEntries = nil
	}

	localFilterListConfig := localFilterListConfig{
		metricNames:   filterlist,
		matchPrefix:   filterlistPrefix,
		tagFilterList: tagFilterListEntries,
	}

	tlmMetricFilterListUpdates := telemetryComp.NewSimpleCounter("filterlist", "updates",
		"Incremented when a reconfiguration of the metric filterlist happened",
	)
	tlmMetricFilterListSize := telemetryComp.NewSimpleGauge("filterlist", "size",
		"Metric filter list size",
	)
	tlmTagFilterListUpdates := telemetryComp.NewSimpleCounter("tag_filterlist", "updates",
		"Incremented when a reconfiguration of the tag filterlist happened",
	)
	tlmTagFilterListSize := telemetryComp.NewSimpleGauge("tag_filterlist", "size",
		"Tag filter list size",
	)

	fl := &FilterList{
		localFilterListConfig:      localFilterListConfig,
		config:                     config,
		log:                        log,
		telemetryComp:              telemetryComp,
		tlmMetricFilterListUpdates: tlmMetricFilterListUpdates,
		tlmMetricFilterListSize:    tlmMetricFilterListSize,
		tlmTagFilterListUpdates:    tlmTagFilterListUpdates,
		tlmTagFilterListSize:       tlmTagFilterListSize,
	}
	compiledTag := loadTagFilterList(localFilterListConfig.tagFilterList, log)
	fl.setTagFilterList(compiledTag)

	fl.SetMetricFilterList(localFilterListConfig.metricNames, localFilterListConfig.matchPrefix)

	return fl
}

// loadTagFilterList loads the tag filterlist from the provided entries.
// Configuration schema is a list of objects with fields:
// - metric_name: the name of the metric
// - action: either "include" or "exclude"
// - tags: array of tags to include/exclude
func loadTagFilterList(entries []MetricTagListEntry, log log.Component) tagMatcher {
	// Build map with merging logic:
	// - If multiple entries have same metric_name and same action: merge tags
	// - If different action: keep only exclude tags (overwrite with exclude)
	tagFilterList := make(map[string]MetricTagList)
	for _, entry := range entries {
		if entry.MetricName == "" {
			log.Warn("skipping metric_tag_filterlist entry with empty metric_name")
			continue
		}

		existing, exists := tagFilterList[entry.MetricName]
		if !exists {
			// First entry for this metric
			tagFilterList[entry.MetricName] = MetricTagList{
				Tags:   entry.Tags,
				Action: entry.Action,
			}
			continue
		}

		// Merge logic
		if existing.Action == entry.Action {
			// Same action: merge tags
			tagFilterList[entry.MetricName] = MetricTagList{
				Tags:   append(existing.Tags, entry.Tags...),
				Action: existing.Action,
			}
		} else if entry.Action == "exclude" {
			// Different actions: keep only exclude tags
			tagFilterList[entry.MetricName] = MetricTagList{
				Tags:   entry.Tags,
				Action: "exclude",
			}
		} else if existing.Action == "exclude" {
			// Keep existing exclude, ignore new include
			continue
		}
	}

	return newTagMatcher(tagFilterList, log)
}

// GetTagFilterList returns the current tag filterlistdef.
func (fl *FilterList) GetTagFilterList() filterlistdef.TagMatcher {
	fl.updateTagMtx.RLock()
	defer fl.updateTagMtx.RUnlock()
	return fl.tagFilterList
}

// GetMetricFilterList returns the current metric filterlist.
func (fl *FilterList) GetMetricFilterList() metricname.Matcher {
	fl.updateMetricMtx.RLock()
	defer fl.updateMetricMtx.RUnlock()
	return fl.filterList
}

// GetHistoFilterList returns the current histogram-specific metric filterlistdef.
// This is a subset of the full metric filterlist containing only entries that
// match histogram aggregate suffixes. It is used by DogStatsD workers which
// pre-filter regular metrics in listeners; only histogram-derived names need
// post-aggregation filtering.
func (fl *FilterList) GetHistoFilterList() metricname.Matcher {
	fl.updateMetricMtx.RLock()
	defer fl.updateMetricMtx.RUnlock()
	return fl.histoFilterList
}

// isHistogramAggregateSuffix reports whether `metricName` ends with a
// configured histogram aggregate or percentile suffix, meaning it can only be
// produced by aggregating a histogram: such a name isn't filterable until
// after aggregation, so it belongs in the histogram-specific filter list.
//
// It is never asked about entries matched by prefix: those always belong in
// the histogram filter list already (see `SetMetricFilterList`), regardless
// of this predicate.
func (fl *FilterList) isHistogramAggregateSuffix(metricName string) bool {
	aggrs := fl.config.GetStringSlice("histogram_aggregates")
	if slices.ContainsFunc(aggrs, func(aggr string) bool {
		return strings.HasSuffix(metricName, "."+aggr)
	}) {
		return true
	}

	percentiles := metrics.ParsePercentiles(fl.config.GetStringSlice("histogram_percentiles"))
	return slices.ContainsFunc(percentiles, func(percentile int) bool {
		return strings.HasSuffix(metricName, fmt.Sprintf(".%dpercentile", percentile))
	})
}

// SetTagFilterList takes a map of metric names to tag configuration, hashes the
// tags and stores the hashed configuration.
func (fl *FilterList) SetTagFilterList(metricTags map[string]MetricTagList) {
	fl.setTagFilterList(newTagMatcher(metricTags, fl.log))
}

func (fl *FilterList) setTagFilterList(metricTags tagMatcher) {
	fl.log.Debugf("SetTagFilterList with %d metrics", len(metricTags.MetricTags))

	fl.tlmTagFilterListUpdates.Inc()
	fl.tlmTagFilterListSize.Set(float64(len(metricTags.MetricTags)))

	fl.updateTagMtx.Lock()
	fl.tagFilterList = metricTags
	fl.updateTagMtx.Unlock()

	fl.updateTagMtx.RLock()
	defer fl.updateTagMtx.RUnlock()

	for _, update := range fl.tagFilterListUpdate {
		update(fl.tagFilterList)
	}
}

// normalizeMetricNames normalizes each entry so it matches the name space the
// matcher compares in, and reports the ones dropped for not being able to match
// any metric name the intake stores. `matchPrefix` makes every entry a prefix,
// whether or not it is written with a trailing `*`.
//
// The entry format and the normalizing itself belong to metricname, which owns
// both the `*` convention and the name space entries are compared in.
func normalizeMetricNames(names []string, matchPrefix bool, log log.Component) []string {
	normalized, dropped := metricname.NormalizeEntries(names, matchPrefix)
	for _, entry := range dropped {
		log.Warnf("metric_filterlist: dropping entry %q that cannot match any metric name stored by Datadog", entry)
	}
	return normalized
}

// SetMetricFilterList updates the metric names filter on all running worker.
// A metric name ending with `*` is a prefix, matching every name starting with
// the rest of the entry. `matchPrefix` turns every entry into a prefix.
func (fl *FilterList) SetMetricFilterList(metricNames []string, matchPrefix bool) {
	fl.log.Debugf("SetMetricFilterList with %d metrics", len(metricNames))

	// we will use two different filterlists:
	// - one with all the metrics names, with all values from `metricNames`
	// - one with only the metric names ending with histogram aggregates suffixes
	//
	// A prefix entry can match any name starting with it, including the
	// aggregates derived from a histogram, so it always belongs in the
	// histogram filter list too: its compiled prefixes are therefore always
	// identical to the main filter list's, and RestrictExact shares them
	// instead of recompiling a duplicate copy.
	filterList := metricname.NewMatcher(metricNames, matchPrefix)
	histoFilterList := filterList.RestrictExact(fl.isHistogramAggregateSuffix)

	// Worth a warning, since it silently drops every metric.
	if filterList.MatchesAll() {
		fl.log.Error("the metric filterlist contains an entry matching every metric name: all metrics will be dropped")
	}

	// Report the compiled size: with prefix matching, NewMatcher compacts
	// redundant sub-prefixes, so len(metricNames) can overcount.
	fl.tlmMetricFilterListUpdates.Inc()
	fl.tlmMetricFilterListSize.Set(float64(filterList.Len()))

	fl.updateMetricMtx.Lock()
	fl.filterList = filterList
	fl.histoFilterList = histoFilterList
	fl.updateMetricMtx.Unlock()

	fl.updateMetricMtx.RLock()
	defer fl.updateMetricMtx.RUnlock()

	for _, update := range fl.metricFilterListUpdate {
		update(fl.filterList, fl.histoFilterList)
	}
}

func (fl *FilterList) restoreMetricFilterListFromLocalConfig() {
	fl.log.Debug("Restoring metric filterlist with local config.")

	fl.SetMetricFilterList(
		fl.localFilterListConfig.metricNames,
		fl.localFilterListConfig.matchPrefix,
	)
}

func (fl *FilterList) restoreTagFilterListFromLocalConfig() {
	fl.log.Debug("Restoring tag metric filterlist with local config.")

	compiled := loadTagFilterList(fl.localFilterListConfig.tagFilterList, fl.log)
	fl.setTagFilterList(compiled)
}

// OnUpdateMetricFilterList is called to register a callback to be called when the
// metric list is updated.
func (fl *FilterList) OnUpdateMetricFilterList(onUpdate func(metricname.Matcher, metricname.Matcher)) {
	fl.updateMetricMtx.Lock()
	fl.metricFilterListUpdate = append(fl.metricFilterListUpdate, onUpdate)
	fl.updateMetricMtx.Unlock()
}

// OnUpdateTagFilterList is called to register a callback to be called when the
// metric tag list is updated.
func (fl *FilterList) OnUpdateTagFilterList(onUpdate func(filterlistdef.TagMatcher)) {
	fl.updateTagMtx.Lock()
	fl.tagFilterListUpdate = append(fl.tagFilterListUpdate, onUpdate)
	fl.updateTagMtx.Unlock()
}

func NewComponent(req Requires) Provides {
	filterList := NewFilterList(req.Log, req.Cfg, req.Telemetry)

	var rcListener rctypes.ListenerProvider
	rcListener.ListenerProvider = rctypes.RCListener{
		state.ProductMetricControl: filterList.onFilterListUpdateCallback,
	}

	return Provides{
		Comp:       filterList,
		RCListener: rcListener,
	}
}
