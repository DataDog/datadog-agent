// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	nodeAgents = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: "cluster_checks",
			Name:      "nodes_reporting",
			Help:      "Number of node agents reporting.",
		},
	)
	danglingConfigs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: "cluster_checks",
			Name:      "configs_dangling",
			Help:      "Number of check configurations not dispatched.",
		},
	)
	dispatchedConfigs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "cluster_checks",
			Name:      "configs_dispatched",
			Help:      "Number of check configurations dispatched, by node.",
		},
		[]string{"node"},
	)
	rebalancingDecisions = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: "cluster_checks",
			Name:      "rebalancing_decisions",
			Help:      "Total number of check rebalancing decisions",
		},
	)
	successfulRebalancing = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: "cluster_checks",
			Name:      "successful_rebalancing_moves",
			Help:      "Total number of successful check rebalancing decisions",
		},
	)
	rebalancingDuration = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: "cluster_checks",
			Name:      "rebalancing_duration_seconds",
			Help:      "Duration of the check rebalancing algorithm last execution",
		},
	)
	statsCollectionFails = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "cluster_checks",
			Name:      "failed_stats_collection",
			Help:      "Total number of unsuccessful stats collection attempts",
		},
		[]string{"node"},
	)
	updateStatsDuration = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: "cluster_checks",
			Name:      "updating_stats_duration_seconds",
			Help:      "Duration of collecting stats from check runners and updating cache",
		},
	)

	allMetrics = []prometheus.Collector{
		nodeAgents,
		danglingConfigs,
		dispatchedConfigs,
		rebalancingDecisions,
		successfulRebalancing,
		rebalancingDuration,
		statsCollectionFails,
		updateStatsDuration,
	}
)

func registerMetrics() {
	for _, m := range allMetrics {
		prometheus.Register(m)
	}
}

func unregisterMetrics() {
	for _, m := range allMetrics {
		prometheus.Unregister(m)
	}
}
