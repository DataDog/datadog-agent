// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package names

import (
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metricpipelines/allowlist"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

const cloudCostOnlyMode = "cloud_cost_only"

type cloudCostMetricsCriterion struct{}

func (cloudCostMetricsCriterion) id() CriterionID {
	return CriterionCloudCostMetrics
}

func (cloudCostMetricsCriterion) active(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetString("infrastructure_mode") == cloudCostOnlyMode &&
		cfg.GetBool("integration.cloud_cost_only.metric_filtering.enabled")
}

func (cloudCostMetricsCriterion) matchers(cfg pkgconfigmodel.Reader, _ filterlist.Component) (utilstrings.Matcher, utilstrings.Matcher) {
	const metricsKey = "integration.cloud_cost_only.metrics"
	matchPrefix := cfg.GetBool("integration.cloud_cost_only.metrics_match_prefix")
	blocked := cfg.GetStringSlice("integration.cloud_cost_only.metrics_blocked")
	allowed := cfg.GetStringSlice(metricsKey)

	var allowMatcher utilstrings.Matcher
	switch {
	case len(allowed) == 0 && cfg.IsConfigured(metricsKey):
		allowMatcher = utilstrings.NewDenyAllAllowlistMatcher()
	case len(allowed) == 0:
		allowed = allowlist.DefaultCloudCostMetrics
		allowMatcher = utilstrings.NewAllowlistMatcher(allowed, matchPrefix)
	default:
		allowMatcher = utilstrings.NewAllowlistMatcher(allowed, matchPrefix)
	}
	return utilstrings.NewBlocklistMatcher(blocked, matchPrefix), allowMatcher
}
