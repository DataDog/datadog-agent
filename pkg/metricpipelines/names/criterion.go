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

// CriterionID identifies a metric-name filter source. Add new IDs when extending criteria.
type CriterionID string

const (
	// CriterionMetricFilterList is metric_filterlist / statsd_metric_blocklist (and RC updates).
	CriterionMetricFilterList CriterionID = "metric_filterlist"
	// CriterionCloudCostMetrics is integration.cloud_cost_only.metrics(_blocked).
	CriterionCloudCostMetrics CriterionID = "cloud_cost_only_metrics"
)

// criterion describes one block/allow source for metric names.
type criterion interface {
	id() CriterionID
	active(cfg pkgconfigmodel.Reader) bool
	matchers(cfg pkgconfigmodel.Reader, filterList filterlist.Component) (blockList, allowList utilstrings.Matcher)
}

// criteria is the registry of metric-name filter sources. Append new criteria here.
var criteria = []criterion{
	metricFilterListCriterion{},
	cloudCostMetricsCriterion{},
}
