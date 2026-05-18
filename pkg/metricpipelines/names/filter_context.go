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
	Name      string
	Source    metrics.MetricSource
	CheckName string // check-sourced metrics, or JMX check from dd.internal.jmx_check_name on DogStatsD
}

// BypassesCloudCostFilter reports whether a metric bypasses cloud_cost_only allowlist
// filtering (DogStatsD, custom_* checks, or integration.additional checks).
func (ctx FilterContext) BypassesCloudCostFilter(additionalChecks []string) bool {
	if ctx.Source == metrics.MetricSourceDogstatsd {
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
