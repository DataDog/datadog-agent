// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// These collectors gather telemetry data for cross-org analysis
// They are not expected to appear in the originiating org's metrics
var (
	// TlmOracleActivityLatency is the time for the activity gathering to complete
	TlmOracleActivityLatency = telemetry.NewHistogram("oracle", "activity_latency", nil, "Histogram of activity query latency in ms", []float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000})
	// TlmOracleActivitySamplesCount is the number of activity samples collected
	TlmOracleActivitySamplesCount = telemetry.NewCounter("oracle", "activity_samples_count", nil, "Number of activity samples collected")
	// TlmOracleStatementMetricsLatency is the time for the statement metrics gathering to complete
	TlmOracleStatementMetricsLatency = telemetry.NewHistogram("oracle", "statement_metrics", nil, "Histogram of statement metrics latency in ms", []float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000})
	// TlmOracleStatementMetricsErrorCount is the number of statement plan errors
	TlmOracleStatementMetricsErrorCount = telemetry.NewCounter("oracle", "statement_plan_errors", nil, "Number of statement plan errors")
)
