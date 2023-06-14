// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
)

var (
	nodeAgents = telemetry.NewGaugeWithOpts("cluster_checks", "nodes_reporting",
		[]string{le.JoinLeaderLabel}, "Number of node agents reporting.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	danglingConfigs = telemetry.NewGaugeWithOpts("cluster_checks", "configs_dangling",
		[]string{le.JoinLeaderLabel}, "Number of check configurations not dispatched.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	dispatchedConfigs = telemetry.NewGaugeWithOpts("cluster_checks", "configs_dispatched",
		[]string{"node", le.JoinLeaderLabel}, "Number of check configurations dispatched, by node.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	dispatchedEndpoints = telemetry.NewGaugeWithOpts("endpoint_checks", "configs_dispatched",
		[]string{"node", le.JoinLeaderLabel}, "Number of endpoint check configurations dispatched, by node.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rebalancingDecisions = telemetry.NewCounterWithOpts("cluster_checks", "rebalancing_decisions",
		[]string{le.JoinLeaderLabel}, "Total number of check rebalancing decisions",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	successfulRebalancing = telemetry.NewCounterWithOpts("cluster_checks", "successful_rebalancing_moves",
		[]string{le.JoinLeaderLabel}, "Total number of successful check rebalancing decisions",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	rebalancingDuration = telemetry.NewGaugeWithOpts("cluster_checks", "rebalancing_duration_seconds",
		[]string{le.JoinLeaderLabel}, "Duration of the check rebalancing algorithm last execution",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	statsCollectionFails = telemetry.NewCounterWithOpts("cluster_checks", "failed_stats_collection",
		[]string{"node", le.JoinLeaderLabel}, "Total number of unsuccessful stats collection attempts",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	updateStatsDuration = telemetry.NewGaugeWithOpts("cluster_checks", "updating_stats_duration_seconds",
		[]string{le.JoinLeaderLabel}, "Duration of collecting stats from check runners and updating cache",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	busyness = telemetry.NewGaugeWithOpts("cluster_checks", "busyness",
		[]string{"node", le.JoinLeaderLabel}, "Busyness of a node per the number of metrics submitted and average duration of all checks run",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	configsInfo = telemetry.NewGaugeWithOpts("cluster_checks", "configs_info",
		[]string{"node", "check_id", le.JoinLeaderLabel}, "Information about the dispatched checks (node, check ID)",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
)
