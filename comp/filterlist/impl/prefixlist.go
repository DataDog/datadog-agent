// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
)

// MetricPrefixListEntry is a single metric_filterlist_prefix entry, as loaded
// from the configuration file or built from a Remote Configuration update. It
// matches every metric name starting with Prefix, except a name matched by
// one of ExceptExact or ExceptPrefix.
//
// Prefix and every entry of ExceptPrefix are prefixes; every entry of
// ExceptExact is a complete metric name. None of them ever carry a `*`
// suffix: unlike metric_filterlist, every entry of metric_filterlist_prefix
// is always a prefix, `*` included, since a literal `*` can never appear in a
// normalized metric name.
type MetricPrefixListEntry struct {
	Prefix       string   `mapstructure:"prefix" yaml:"prefix" json:"prefix"`
	ExceptPrefix []string `mapstructure:"except_prefix" yaml:"except_prefix" json:"except_prefix"`
	ExceptExact  []string `mapstructure:"except_exact" yaml:"except_exact" json:"except_exact"`
}

// normalizeMetricPrefixList converts metric_filterlist_prefix entries into
// metricname.PrefixRule values ready for SetMetricFilterList, normalizing
// them and logging every rule or exception dropped for not being able to
// match any metric name stored by Datadog.
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
