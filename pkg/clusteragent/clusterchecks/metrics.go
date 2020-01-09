// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	nodeAgents = telemetry.NewGauge("cluster_checks", "nodes_reporting",
		nil, "Number of node agents reporting.")
	danglingConfigs = telemetry.NewGauge("cluster_checks", "configs_dangling",
		nil, "Number of check configurations not dispatched.")
	dispatchedConfigs = telemetry.NewGauge("cluster_checks", "configs_dispatched",
		[]string{"node"}, "Number of check configurations dispatched, by node.")
	rebalancingDecisions = telemetry.NewCounter("cluster_checks", "rebalancing_decisions",
		nil, "Total number of check rebalancing decisions")
	successfulRebalancing = telemetry.NewCounter("cluster_checks", "successful_rebalancing_moves",
		nil, "Total number of successful check rebalancing decisions")
	rebalancingDuration = telemetry.NewGauge("cluster_checks", "rebalancing_duration_seconds",
		nil, "Duration of the check rebalancing algorithm last execution")
	statsCollectionFails = telemetry.NewCounter("cluster_checks", "failed_stats_collection",
		[]string{"node"}, "Total number of unsuccessful stats collection attempts")
	updateStatsDuration = telemetry.NewGauge("cluster_checks", "updating_stats_duration_seconds",
		nil, "Duration of collecting stats from check runners and updating cache")
)
