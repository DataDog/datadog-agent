// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics provides telemetry to know who's the leader in Kubernetes
// objects that implement the leader/follower pattern.
package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	// JoinLeaderLabel represents the join_leader label. It's a static label for label joins (always true)
	JoinLeaderLabel = "join_leader"
	// JoinLeaderValue is the static value of the label join_leader
	JoinLeaderValue = "true"
	// isLeaderLabel represents the is_leader label
	isLeaderLabel = "is_leader"
)

// NewLeaderMetric returns the leader_election_is_leader metric
func NewLeaderMetric() telemetry.Gauge {
	return telemetry.NewGaugeWithOpts(
		"leader_election",
		"is_leader",
		[]string{JoinLeaderLabel, isLeaderLabel}, // join_leader is for label joins
		"The label is_leader is true if the reporting pod is leader, equals false otherwise.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
}
