// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterlistimpl provides the implementation for the filterlist/rc component
package filterlistimpl

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
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
	metricNames []string
	matchPrefix bool
}

type FilterList struct {
	localFilterListConfig

	log           log.Component
	config        config.Component
	telemetrycomp telemetry.Component

	filterListUpdate []func(*utilstrings.Matcher, *utilstrings.Matcher)
	filterList       utilstrings.Matcher
	histoFilterList  utilstrings.Matcher

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

	tlmFilterListUpdates := telemetrycomp.NewSimpleCounter("filterlist", "filterlist_updates",
		"Incremented when a reconfiguration of the filterlist happened",
	)
	tlmFilterListSize := telemetrycomp.NewSimpleGauge("filterlist", "filterlist_size",
		"Filter list size",
	)

	fl := &FilterList{
		localFilterListConfig: localFilterListConfig,
		config:                config,
		log:                   log,
		telemetrycomp:         telemetrycomp,
		tlmFilterListUpdates:  tlmFilterListUpdates,
		tlmFilterListSize:     tlmFilterListSize,
	}

	fl.SetFilterList(localFilterListConfig.metricNames, localFilterListConfig.matchPrefix)

	return fl
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

	for _, update := range fl.filterListUpdate {
		update(&fl.filterList, &fl.histoFilterList)
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

func (fl *FilterList) OnUpdateMetricFilterList(onUpdate func(*utilstrings.Matcher, *utilstrings.Matcher)) {
	fl.filterListUpdate = append(fl.filterListUpdate, onUpdate)
	onUpdate(&fl.filterList, &fl.histoFilterList)
}

func NewRCReq(req Requires) Provides {
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
