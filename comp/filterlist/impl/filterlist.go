// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterlistimpl provides the implementation for the filterlist/rc component
package filterlistimpl

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// Requires contains the config for RC
type Requires struct {
	Cfg       config.Component
	Log       log.Component
	Telemetry telemetry.Component
}

// Provides contains the RC component
type Provides struct {
	Comp       filterlist.Component
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
	telemetrycomp telemetry.Component

	updateMetricMtx        sync.RWMutex
	metricFilterListUpdate []func(utilstrings.Matcher, utilstrings.Matcher)
	filterList             utilstrings.Matcher
	histoFilterList        utilstrings.Matcher

	updateTagMtx        sync.RWMutex
	tagFilterListUpdate []func(filterlist.TagMatcher)
	tagFilterList       tagMatcher

	tlmMetricFilterListUpdates telemetry.SimpleCounter
	tlmMetricFilterListSize    telemetry.SimpleGauge

	tlmTagFilterListUpdates telemetry.SimpleCounter
	tlmTagFilterListSize    telemetry.SimpleGauge
}

// Load the config, registers with RC
func NewFilterList(log log.Component, config config.Component, telemetrycomp telemetry.Component) *FilterList {
	// init the metric names filterlist
	filterlist := config.GetStringSlice("metric_filterlist")
	filterlistPrefix := config.GetBool("metric_filterlist_match_prefix")
	if len(filterlist) == 0 {
		filterlist = config.GetStringSlice("statsd_metric_blocklist")
		filterlistPrefix = config.GetBool("statsd_metric_blocklist_match_prefix")
	}

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

	tlmMetricFilterListUpdates := telemetrycomp.NewSimpleCounter("filterlist", "updates",
		"Incremented when a reconfiguration of the metric filterlist happened",
	)
	tlmMetricFilterListSize := telemetrycomp.NewSimpleGauge("filterlist", "size",
		"Metric filter list size",
	)
	tlmTagFilterListUpdates := telemetrycomp.NewSimpleCounter("tag_filterlist", "updates",
		"Incremented when a reconfiguration of the tag filterlist happened",
	)
	tlmTagFilterListSize := telemetrycomp.NewSimpleGauge("tag_filterlist", "size",
		"Tag filter list size",
	)

	fl := &FilterList{
		localFilterListConfig:      localFilterListConfig,
		config:                     config,
		log:                        log,
		telemetrycomp:              telemetrycomp,
		tlmMetricFilterListUpdates: tlmMetricFilterListUpdates,
		tlmMetricFilterListSize:    tlmMetricFilterListSize,
		tlmTagFilterListUpdates:    tlmTagFilterListUpdates,
		tlmTagFilterListSize:       tlmTagFilterListSize,
	}
	fl.SetTagFilterListFromEntries(localFilterListConfig.tagFilterList)
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

	return newTagMatcher(tagFilterList)
}

// GetTagFilterList returns the current tag filterlist.
func (fl *FilterList) GetTagFilterList() filterlist.TagMatcher {
	return &fl.tagFilterList
}

// GetMetricFilterList returns the current metric filterlist.
func (fl *FilterList) GetMetricFilterList() utilstrings.Matcher {
	return fl.filterList
}

// create a list based on all `metricNames` but only containing metric names
// with histogram aggregates suffixes.
func (fl *FilterList) createHistogramsFilterList(metricNames []string) []string {
	aggrs := fl.config.GetStringSlice("histogram_aggregates")

	percentiles := metrics.ParsePercentiles(fl.config.GetStringSlice("histogram_percentiles"))
	percentileAggrs := make([]string, len(percentiles))
	for i, percentile := range percentiles {
		percentileAggrs[i] = fmt.Sprintf("%dpercentile", percentile)
	}

	histoMetricNames := []string{}
	for _, metricName := range metricNames {
		// metric names ending with a histogram aggregates
		for _, aggr := range aggrs {
			if strings.HasSuffix(metricName, "."+aggr) {
				histoMetricNames = append(histoMetricNames, metricName)
			}
		}
		// metric names ending with a percentile
		for _, percentileAggr := range percentileAggrs {
			if strings.HasSuffix(metricName, "."+percentileAggr) {
				histoMetricNames = append(histoMetricNames, metricName)
			}
		}
	}

	fl.log.Debugf("SetMetricFilterList created a histograms subsets of %d metric names", len(histoMetricNames))
	return histoMetricNames
}

// SetTagFilterList takes a map of metric names to tag configuration, hashes the
// tags and stores the hashed configuration.
func (fl *FilterList) SetTagFilterList(metricTags map[string]MetricTagList) {
	fl.setTagFilterList(newTagMatcher(metricTags))
}

func (fl *FilterList) setTagFilterList(metricTags tagMatcher) {
	fl.log.Debugf("SetTagFilterList with %d metrics", len(metricTags.MetricTags))

	fl.updateTagMtx.Lock()
	fl.tagFilterList = metricTags
	fl.updateTagMtx.Unlock()

	fl.updateTagMtx.RLock()
	defer fl.updateTagMtx.RUnlock()

	for _, update := range fl.tagFilterListUpdate {
		update(&fl.tagFilterList)
	}
}

// SetTagFilterListFromEntries takes a list of tag filter list objects that
// were loaded from the config file, converts and hashes the tags in a format
// used internally. Any registered callbacks are informed of the update.
func (fl *FilterList) SetTagFilterListFromEntries(entries []MetricTagListEntry) {
	fl.setTagFilterList(loadTagFilterList(entries, fl.log))
}

// SetMetricFilterList updates the metric names filter on all running worker.
func (fl *FilterList) SetMetricFilterList(metricNames []string, matchPrefix bool) {
	fl.log.Debugf("SetMetricFilterList with %d metrics", len(metricNames))

	// we will use two different filterlists:
	// - one with all the metrics names, with all values from `metricNames`
	// - one with only the metric names ending with histogram aggregates suffixes

	// only histogram metric names (including their aggregates suffixes)
	histoMetricNames := fl.createHistogramsFilterList(metricNames)
	filterList := utilstrings.NewMatcher(metricNames, matchPrefix)
	histoFilterList := utilstrings.NewMatcher(histoMetricNames, matchPrefix)

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

	fl.tlmMetricFilterListUpdates.Inc()
	fl.tlmMetricFilterListSize.Set(float64(len(fl.localFilterListConfig.metricNames)))

	fl.SetMetricFilterList(
		fl.localFilterListConfig.metricNames,
		fl.localFilterListConfig.matchPrefix,
	)
}

func (fl *FilterList) restoreTagFilterListFromLocalConfig() {
	fl.log.Debug("Restoring tag metric filterlist with local config.")

	fl.tlmTagFilterListUpdates.Inc()
	fl.tlmTagFilterListSize.Set(float64(len(fl.localFilterListConfig.tagFilterList)))

	fl.SetTagFilterListFromEntries(fl.localFilterListConfig.tagFilterList)
}

// OnUpdateMetricFilterList is called to register a callback to be called when the
// metric list is updated.
func (fl *FilterList) OnUpdateMetricFilterList(onUpdate func(utilstrings.Matcher, utilstrings.Matcher)) {
	fl.updateMetricMtx.Lock()
	fl.metricFilterListUpdate = append(fl.metricFilterListUpdate, onUpdate)
	fl.updateMetricMtx.Unlock()

	fl.updateMetricMtx.RLock()
	defer fl.updateMetricMtx.RUnlock()

	onUpdate(fl.filterList, fl.histoFilterList)
}

// OnUpdateTagFilterList is called to register a callback to be called when the
// metric tag list is updated.
func (fl *FilterList) OnUpdateTagFilterList(onUpdate func(filterlist.TagMatcher)) {
	fl.updateTagMtx.Lock()
	fl.tagFilterListUpdate = append(fl.tagFilterListUpdate, onUpdate)
	fl.updateTagMtx.Unlock()

	fl.updateTagMtx.RLock()
	defer fl.updateTagMtx.RUnlock()

	onUpdate(&fl.tagFilterList)
}

func NewFilterListReq(req Requires) Provides {
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
