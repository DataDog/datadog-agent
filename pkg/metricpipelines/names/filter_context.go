// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package names

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// FilterContext carries fields used by metric-name filters beyond the metric name.
type FilterContext struct {
	Name          string
	Source        metrics.MetricSource
	CheckName     string // integration check name (check_sampler only)
	FromDogstatsd bool   // set on DogStatsD ingest (including JMX-tagged metrics)
}

// BypassesCloudCostFilter reports whether a metric bypasses cloud_cost_only allowlist
// filtering (DogStatsD ingest, custom_* checks, or integration.additional checks).
func (ctx FilterContext) BypassesCloudCostFilter(additionalChecks []string) bool {
	if ctx.FromDogstatsd {
		return true
	}
	if ctx.CheckName == "" {
		return false
	}
	if strings.HasPrefix(ctx.CheckName, "custom_") {
		return true
	}
	return slices.Contains(additionalChecks, ctx.CheckName)
}
