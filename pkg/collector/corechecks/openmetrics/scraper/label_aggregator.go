// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// LabelAggregator implements the share_labels / label_joins feature.
// It enriches metrics with labels from configured "source" metrics that share
// common match labels.
type LabelAggregator struct {
	configs      map[string]ShareLabelsConfig
	cache        map[string][]labelMapping
	cacheEnabled bool
	populated    bool
}

// labelMapping stores the match key values and the extra tags to share.
type labelMapping struct {
	matchKey  string   // concatenated match label values, e.g. "namespace=default,pod=nginx"
	extraTags []string // tags to share, e.g. ["node:worker-1", "host_ip:10.0.0.1"]
}

// NewLabelAggregator creates a LabelAggregator from the share_labels config.
func NewLabelAggregator(shareLabels map[string]ShareLabelsConfig, cacheEnabled bool) *LabelAggregator {
	return &LabelAggregator{
		configs:      shareLabels,
		cache:        make(map[string][]labelMapping),
		cacheEnabled: cacheEnabled,
	}
}

// Process collects labels from source metrics. Call once per scrape before
// submitting metrics. It scans all metric families for configured source metric
// names and indexes their labels.
func (la *LabelAggregator) Process(families []prometheus.MetricFamily) {
	if la.cacheEnabled && la.populated {
		return
	}

	for sourceMetric, cfg := range la.configs {
		var family *prometheus.MetricFamily
		for i := range families {
			if families[i].Name == sourceMetric {
				family = &families[i]
				break
			}
		}
		if family == nil {
			continue
		}

		// Sort match labels once for deterministic key building.
		sortedMatch := make([]string, len(cfg.Match))
		copy(sortedMatch, cfg.Match)
		sort.Strings(sortedMatch)

		// Build a set of labels to share, if explicitly configured.
		var labelsToShare map[string]struct{}
		if len(cfg.Labels) > 0 {
			labelsToShare = make(map[string]struct{}, len(cfg.Labels))
			for _, l := range cfg.Labels {
				labelsToShare[l] = struct{}{}
			}
		}

		// Build a set of match labels for quick exclusion when collecting extras.
		matchSet := make(map[string]struct{}, len(sortedMatch))
		for _, m := range sortedMatch {
			matchSet[m] = struct{}{}
		}

		mappings := make([]labelMapping, 0, len(family.Samples))
		for _, sample := range family.Samples {
			// Build the match key from the sample's labels.
			matchKey := buildMatchKey(sample.Metric, sortedMatch)

			// Collect extra tags.
			var extraTags []string
			if labelsToShare != nil {
				// Only collect specified labels.
				extraTags = make([]string, 0, len(labelsToShare))
				for _, lbl := range cfg.Labels {
					if v, ok := sample.Metric[lbl]; ok {
						extraTags = append(extraTags, lbl+":"+v)
					}
				}
			} else {
				// Collect all labels except __name__ and match labels.
				extraTags = make([]string, 0, len(sample.Metric))
				for k, v := range sample.Metric {
					if k == "__name__" {
						continue
					}
					if _, isMatch := matchSet[k]; isMatch {
						continue
					}
					extraTags = append(extraTags, k+":"+v)
				}
			}

			mappings = append(mappings, labelMapping{
				matchKey:  matchKey,
				extraTags: extraTags,
			})
		}

		la.cache[sourceMetric] = mappings
	}

	la.populated = true
}

// GetSharedTags returns extra tags to append to a sample based on its labels
// matching source metrics.
func (la *LabelAggregator) GetSharedTags(sampleLabels map[string]string) []string {
	var result []string

	for sourceMetric, cfg := range la.configs {
		mappings, ok := la.cache[sourceMetric]
		if !ok {
			continue
		}

		// Sort match labels for deterministic key building (same order as Process).
		sortedMatch := make([]string, len(cfg.Match))
		copy(sortedMatch, cfg.Match)
		sort.Strings(sortedMatch)

		targetKey := buildMatchKey(sampleLabels, sortedMatch)

		for _, m := range mappings {
			if m.matchKey == targetKey {
				result = append(result, m.extraTags...)
				break
			}
		}
	}

	return result
}

// buildMatchKey builds a deterministic key from the given labels map using
// the provided sorted label names. The result is a comma-separated list of
// "key=value" pairs, e.g. "namespace=default,pod=nginx".
func buildMatchKey(labels map[string]string, sortedMatchLabels []string) string {
	parts := make([]string, 0, len(sortedMatchLabels))
	for _, k := range sortedMatchLabels {
		v := labels[k]
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}
