// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
)

// MetricPrefixListEntry is a metric_filterlist_prefix entry.
// Prefix and ExceptPrefix are prefixes; ExceptExact is a full metric name.
type MetricPrefixListEntry struct {
	Prefix       string   `mapstructure:"prefix" yaml:"prefix" json:"prefix"`
	ExceptPrefix []string `mapstructure:"except_prefix" yaml:"except_prefix" json:"except_prefix"`
	ExceptExact  []string `mapstructure:"except_exact" yaml:"except_exact" json:"except_exact"`
}

// normalizeMetricPrefixList normalizes entries for SetMetricFilterList.
func normalizeMetricPrefixList(entries []MetricPrefixListEntry, log log.Component) []metricname.PrefixRule {
	rules := make([]metricname.PrefixRule, 0, len(entries))
	for _, entry := range entries {
		rules = append(rules, metricname.PrefixRule{
			Prefix:       entry.Prefix,
			ExceptExact:  entry.ExceptExact,
			ExceptPrefix: entry.ExceptPrefix,
		})
	}

	normalized, droppedRules, droppedExceptions := metricname.NormalizePrefixRules(rules)
	for _, prefix := range droppedRules {
		log.Warnf("metric_filterlist_prefix: dropping entry %q that cannot match any metric name stored by Datadog", prefix)
	}
	for _, exception := range droppedExceptions {
		log.Warnf("metric_filterlist_prefix: dropping exception %q that cannot match any metric name stored by Datadog", exception)
	}
	return normalized
}
