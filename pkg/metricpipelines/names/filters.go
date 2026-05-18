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
	id        CriterionID
	blockList utilstrings.Matcher
	allowList utilstrings.Matcher
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
		rules = append(rules, rule{
			id:        c.id(),
			blockList: blockList,
			allowList: allowList,
		})
	}
	return Filters{rules: rules}
}

// ShouldDrop reports whether a metric name should be dropped according to any active criterion.
func (f Filters) ShouldDrop(name string) bool {
	for i := range f.rules {
		r := &f.rules[i]
		if utilstrings.ShouldDropMetric(name, &r.blockList, &r.allowList) {
			return true
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
