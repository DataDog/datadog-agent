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
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
	"gopkg.in/yaml.v2"
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
	metricNames []string
	matchPrefix bool
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
	tagFilterListUpdate []func(TagMatcher)
	tagFilterList       TagMatcher

	tlmFilterListUpdates telemetry.SimpleCounter
	tlmFilterListSize    telemetry.SimpleGauge
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

	localFilterListConfig := localFilterListConfig{
		metricNames: filterlist,
		matchPrefix: filterlistPrefix,
	}

	tlmFilterListUpdates := telemetrycomp.NewSimpleCounter("filterlist", "updates",
		"Incremented when a reconfiguration of the filterlist happened",
	)
	tlmFilterListSize := telemetrycomp.NewSimpleGauge("filterlist", "size",
		"Filter list size",
	)

	tagMatcher := loadTagFilterList()

	fl := &FilterList{
		localFilterListConfig: localFilterListConfig,
		config:                config,
		log:                   log,
		telemetrycomp:         telemetrycomp,
		tlmFilterListUpdates:  tlmFilterListUpdates,
		tlmFilterListSize:     tlmFilterListSize,

		tagFilterList: tagMatcher,
	}

	fl.SetFilterList(localFilterListConfig.metricNames, localFilterListConfig.matchPrefix)

	return fl
}

// loadTagFilterList loads the tag filterlist from the config file.
func loadTagFilterList(config config.Component, log log.Component) TagMatcher {
	tagFilterListInterface := config.Datadog().GetStringMap("metric_tag_filterlist")

	tagFilterList := make(map[string]MetricTagList, len(tagFilterListInterface))
	for metricName, tags := range tagFilterListInterface {
		// Tags can be configured as an object with fields:
		// tags - array of tags
		// action - either `include` or `exclude`.
		// Roundtrip the struct through yaml to load it.
		tagBytes, err := yaml.Marshal(tags)
		if err != nil {
			log.Errorf("invalid configuration for %q: %s", "metric_tag_filterlist."+metricName, err)
		} else {
			var tags MetricTagList
			err = yaml.Unmarshal(tagBytes, &tags)
			if err != nil {
				log.Errorf("error loading configuration for %q: %s", "metric_tag_filterlist."+metricName, err)
			} else {
				tagFilterList[metricName] = tags
			}
		}
	}
	return NewTagMatcher(tagFilterList)
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

	fl.log.Debugf("SetFilterList created a histograms subsets of %d metric names", len(histoMetricNames))
	return histoMetricNames
}

// SetFilterList updates the metric names filter on all running worker.
func (fl *FilterList) SetFilterList(metricNames []string, matchPrefix bool) {
	fl.log.Debugf("SetFilterList with %d metrics", len(metricNames))

	// we will use two different filterlists:
	// - one with all the metrics names, with all values from `metricNames`
	// - one with only the metric names ending with histogram aggregates suffixes

	// only histogram metric names (including their aggregates suffixes)
	histoMetricNames := fl.createHistogramsFilterList(metricNames)
	fl.filterList = utilstrings.NewMatcher(metricNames, matchPrefix)
	fl.histoFilterList = utilstrings.NewMatcher(histoMetricNames, matchPrefix)

	fl.updateMetricMtx.RLock()
	defer fl.updateMetricMtx.RUnlock()

	for _, update := range fl.filterListUpdate {
		update(fl.filterList, fl.histoFilterList)
	}
}

func (fl *FilterList) restoreFilterListFromLocalConfig() {
	fl.log.Debug("Restoring filterlist with local config.")

	fl.tlmFilterListUpdates.Inc()
	fl.tlmFilterListSize.Set(float64(len(fl.localFilterListConfig.metricNames)))

	fl.SetFilterList(
		fl.localFilterListConfig.metricNames,
		fl.localFilterListConfig.matchPrefix,
	)
}

func (fl *FilterList) OnUpdateMetricFilterList(onUpdate func(utilstrings.Matcher, utilstrings.Matcher)) {
	fl.updateMetricMtx.Lock()
	defer fl.updateMetricMtx.Unlock()

	fl.metricFilterListUpdate = append(fl.metricFilterListUpdate, onUpdate)
	onUpdate(fl.filterList, fl.histoFilterList)
}

func (fl *FilterList) OnUpdateTagFilterList(onUpdate func(TagMatcher)) {
	fl.updateTagMtx.Lock()
	defer fl.updateTagMtx.Unlock()

	fl.tagFilterListUpdate = append(fl.tagFilterListUpdate, onUpdate)
	onUpdate(fl.tagFilterList)
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
