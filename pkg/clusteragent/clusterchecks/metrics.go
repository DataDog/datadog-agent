// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
)

func init() {
	prometheus.MustRegister(nodeAgents)
	prometheus.MustRegister(danglingConfigs)
	prometheus.MustRegister(dispatchedConfigs)
}
