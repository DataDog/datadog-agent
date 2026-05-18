// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package names

import (
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

const cloudCostOnlyMode = "cloud_cost_only"

type cloudCostMetricsCriterion struct{}

func (cloudCostMetricsCriterion) id() CriterionID {
	return CriterionCloudCostMetrics
}

func (cloudCostMetricsCriterion) active(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetString("infrastructure_mode") == cloudCostOnlyMode
}

func (cloudCostMetricsCriterion) matchers(cfg pkgconfigmodel.Reader, _ filterlist.Component) (utilstrings.Matcher, utilstrings.Matcher) {
	matchPrefix := cfg.GetBool("integration.cloud_cost_only.metrics_match_prefix")
	blocked := cfg.GetStringSlice("integration.cloud_cost_only.metrics_blocked")
	allowed := cfg.GetStringSlice("integration.cloud_cost_only.metrics")
	return utilstrings.NewBlocklistMatcher(blocked, matchPrefix), utilstrings.NewAllowlistMatcher(allowed, matchPrefix)
}
