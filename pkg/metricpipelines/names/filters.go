// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package names is the single place to join metric-name block/allow criteria applied at
// flush and DogStatsD ingest. Add new criteria in criterion.go and implement criterion.
package names

import (
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

type rule struct {
	id               CriterionID
	blockList        utilstrings.Matcher
	allowList        utilstrings.Matcher
	additionalChecks []string // integration.additional checks bypass cloud_cost_only allowlist
}

// Filters holds the active metric-name filter criteria for the current config.
type Filters struct {
	rules []rule
}

// Load builds Filters from all registered criteria that are active for cfg.
func Load(cfg pkgconfigmodel.Reader, filterList filterlist.Component) Filters {
	var rules []rule
	for _, c := range criteria {
		if !c.active(cfg) {
			continue
		}
		blockList, allowList := c.matchers(cfg, filterList)
		r := rule{
			id:        c.id(),
			blockList: blockList,
			allowList: allowList,
		}
		if c.id() == CriterionCloudCostMetrics {
			r.additionalChecks = cfg.GetStringSlice("integration.additional")
		}
		rules = append(rules, r)
	}
	return Filters{rules: rules}
}

// ShouldDrop reports whether a metric should be dropped according to any active criterion.
func (f Filters) ShouldDrop(ctx FilterContext) bool {
	for i := range f.rules {
		r := &f.rules[i]
		switch r.id {
		case CriterionCloudCostMetrics:
			if shouldDropCloudCost(ctx, r.blockList, r.allowList, r.additionalChecks) {
				return true
			}
		default:
			if r.blockList.ShouldDrop(ctx.Name) || r.allowList.ShouldDrop(ctx.Name) {
				return true
			}
		}
	}
	return false
}

// SetBlockList updates the blocklist for a criterion (used when metric_filterlist is reconfigured at runtime).
func (f *Filters) SetBlockList(id CriterionID, blockList utilstrings.Matcher) {
	for i := range f.rules {
		if f.rules[i].id == id {
			f.rules[i].blockList = blockList
			return
		}
	}
}

// NewTestFilters builds Filters with a single criterion. For tests and benchmarks only.
func NewTestFilters(id CriterionID, blockList, allowList utilstrings.Matcher) Filters {
	return Filters{rules: []rule{{id: id, blockList: blockList, allowList: allowList}}}
}

// NewTestCloudCostFilters builds cloud-cost Filters for tests. For tests only.
func NewTestCloudCostFilters(blockList, allowList utilstrings.Matcher, additionalChecks []string) Filters {
	return Filters{rules: []rule{{
		id:               CriterionCloudCostMetrics,
		blockList:        blockList,
		allowList:        allowList,
		additionalChecks: additionalChecks,
	}}}
}
