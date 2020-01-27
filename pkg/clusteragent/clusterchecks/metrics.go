// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	nodeAgents = telemetry.NewGaugeWithOpts("cluster_checks", "nodes_reporting",
		nil, "Number of node agents reporting.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	danglingConfigs = telemetry.NewGaugeWithOpts("cluster_checks", "configs_dangling",
		nil, "Number of check configurations not dispatched.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	dispatchedConfigs = telemetry.NewGaugeWithOpts("cluster_checks", "configs_dispatched",
		[]string{"node"}, "Number of check configurations dispatched, by node.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rebalancingDecisions = telemetry.NewCounterWithOpts("cluster_checks", "rebalancing_decisions",
		nil, "Total number of check rebalancing decisions",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	successfulRebalancing = telemetry.NewCounterWithOpts("cluster_checks", "successful_rebalancing_moves",
		nil, "Total number of successful check rebalancing decisions",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rebalancingDuration = telemetry.NewGaugeWithOpts("cluster_checks", "rebalancing_duration_seconds",
		nil, "Duration of the check rebalancing algorithm last execution",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	statsCollectionFails = telemetry.NewCounterWithOpts("cluster_checks", "failed_stats_collection",
		[]string{"node"}, "Total number of unsuccessful stats collection attempts",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	updateStatsDuration = telemetry.NewGaugeWithOpts("cluster_checks", "updating_stats_duration_seconds",
		nil, "Duration of collecting stats from check runners and updating cache",
		telemetry.Options{NoDoubleUnderscoreSep: true})
)
