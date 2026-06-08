// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package lookbackdef defines the lookback ring buffer component interface.
package lookbackdef

// team: agent-metric-pipelines

import (
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// Component captures raw pre-aggregation metric samples and enables replay at
// sub-flush granularity for any time window within the configured retention period.
//
// RecordSample is called by the shadow check pipeline for every metric emitted by
// a shadow check.  Implementations must be safe for concurrent use.
type Component interface {
	RecordSample(id checkid.ID, name string, value float64, tags []string, host string, ts float64, mType metrics.MetricType)
}
