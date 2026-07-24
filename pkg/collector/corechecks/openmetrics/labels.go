// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
)

type labelAggregator struct {
	configured        bool
	cacheSharedLabels bool
	sharedLabelsCache bool
	partialMatches    bool

	metricConfig     map[string]shareLabelConfig
	targetInfo       bool
	targetInfoLabels map[string]string

	labelSets           []sharedLabelSet
	unconditionalLabels map[string]string
}

type shareLabelConfig struct {
	labels map[string]struct{}
	match  map[string]struct{}
	values map[float64]struct{}
}

type sharedLabelSet struct {
	matchLabels  map[string]string
	sharedLabels map[string]string
}

type labelAggregatorPreparer struct {
	aggregator *labelAggregator
	pending    map[string]shareLabelConfig
}

func newLabelAggregator(cfg *scraperConfig) (*labelAggregator, error) {
	aggregator := &labelAggregator{
		cacheSharedLabels:   cfg.cacheSharedLabels,
		partialMatches:      cfg.mode == latestMode,
		metricConfig:        map[string]shareLabelConfig{},
		targetInfo:          cfg.targetInfo,
		targetInfoLabels:    map[string]string{},
		unconditionalLabels: map[string]string{},
	}

	for metric, rawConfig := range cfg.shareLabels {
		shareConfig, err := parseShareLabelConfig(metric, rawConfig)
		if err != nil {
			return nil, err
		}
		aggregator.metricConfig[metric] = shareConfig
	}

	aggregator.configured = len(aggregator.metricConfig) > 0 || aggregator.targetInfo
	return aggregator, nil
}

func parseShareLabelConfig(metric string, cfg types.ShareLabelsConfig) (shareLabelConfig, error) {
	parsed := shareLabelConfig{}
	if len(cfg.Labels) > 0 {
		parsed.labels = stringSet(cfg.Labels)
	}
	if len(cfg.Match) > 0 {
		parsed.match = stringSet(cfg.Match)
	}
	if len(cfg.Values) > 0 {
		parsed.values = make(map[float64]struct{}, len(cfg.Values))
		for i, value := range cfg.Values {
			integer, err := strconv.Atoi(value)
			if err != nil {
				return parsed, fmt.Errorf("entry #%d of option `values` for metric `%s` of setting `share_labels` must represent an integer", i+1, metric)
			}
			parsed.values[float64(integer)] = struct{}{}
		}
	}
	return parsed, nil
}

func (a *labelAggregator) needsPrepass() bool {
	return a.configured && !a.cacheSharedLabels
}

func (a *labelAggregator) newPreparer() *labelAggregatorPreparer {
	a.labelSets = nil
	a.unconditionalLabels = map[string]string{}

	pending := make(map[string]shareLabelConfig, len(a.metricConfig))
	for metric, config := range a.metricConfig {
		pending[metric] = config
	}
	if a.targetInfo {
		pending["target_info"] = shareLabelConfig{}
	}
	return &labelAggregatorPreparer{aggregator: a, pending: pending}
}

func (p *labelAggregatorPreparer) collect(metric parsedMetric) bool {
	config, ok := p.pending[metric.Name]
	if !ok {
		return len(p.pending) == 0
	}
	p.aggregator.collect(metric, config)
	delete(p.pending, metric.Name)
	return len(p.pending) == 0
}

func (a *labelAggregator) beforeMetric(metric parsedMetric) {
	if !a.configured || !a.cacheSharedLabels {
		return
	}

	if !a.sharedLabelsCache {
		if config, ok := a.metricConfig[metric.Name]; ok {
			a.collect(metric, config)
		}
	}
	if a.targetInfo && metric.Name == "target_info" {
		a.collectTargetInfo(metric)
	}
}

func (a *labelAggregator) afterScrape() {
	if !a.configured {
		return
	}
	if a.cacheSharedLabels {
		a.sharedLabelsCache = true
		return
	}

	a.labelSets = nil
	a.unconditionalLabels = map[string]string{}
}

func (a *labelAggregator) collect(metric parsedMetric, config shareLabelConfig) {
	if len(config.match) > 0 {
		for _, sample := range metric.Samples {
			if !valueAllowed(sample.Value, config.values) {
				continue
			}
			matchLabels := map[string]string{}
			sharedLabels := map[string]string{}
			for label, value := range sample.Labels {
				if _, ok := config.match[label]; ok {
					matchLabels[label] = value
				}
				if len(config.labels) == 0 {
					sharedLabels[label] = value
				} else if _, ok := config.labels[label]; ok {
					sharedLabels[label] = value
				}
			}
			if !a.partialMatches {
				matchedAllLabels := true
				for label := range config.match {
					if _, ok := sample.Labels[label]; !ok {
						matchedAllLabels = false
						break
					}
				}
				if !matchedAllLabels {
					continue
				}
			}
			a.labelSets = append(a.labelSets, sharedLabelSet{matchLabels: matchLabels, sharedLabels: sharedLabels})
		}
		return
	}

	for _, sample := range metric.Samples {
		if !valueAllowed(sample.Value, config.values) {
			continue
		}
		for label, value := range sample.Labels {
			if label == nameLabel {
				continue
			}
			if len(config.labels) == 0 {
				if metric.Name == "target_info" {
					if label != nameLabel {
						a.targetInfoLabels[label] = value
					}
				} else {
					a.unconditionalLabels[label] = value
				}
			} else if _, ok := config.labels[label]; ok {
				a.unconditionalLabels[label] = value
			}
		}
	}
}

func (a *labelAggregator) collectTargetInfo(metric parsedMetric) {
	a.targetInfoLabels = map[string]string{}
	a.collect(metric, shareLabelConfig{})
}

func (a *labelAggregator) populate(labels map[string]string) {
	if !a.configured {
		return
	}

	original := copyLabels(labels)
	for label, value := range a.unconditionalLabels {
		labels[label] = value
	}
	for label, value := range a.targetInfoLabels {
		labels[label] = value
	}

	for _, labelSet := range a.labelSets {
		if labelsMatch(original, labelSet.matchLabels) {
			for label, value := range labelSet.sharedLabels {
				labels[label] = value
			}
		}
	}
}

func valueAllowed(value float64, allowed map[float64]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[value]
	return ok
}

func labelsMatch(labels, expected map[string]string) bool {
	for label, value := range expected {
		if labels[label] != value {
			return false
		}
	}
	return true
}

func copyLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func canonicalizeNumericLabel(label string) string {
	value, err := strconv.ParseFloat(label, 64)
	if err != nil {
		return label
	}
	if value == 0 {
		return "0"
	}
	if math.IsInf(value, 1) {
		return "inf"
	}
	if math.IsInf(value, -1) {
		return "-inf"
	}
	if value == math.Trunc(value) {
		return strconv.FormatFloat(value, 'f', 1, 64)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func normalizeSampleLabels(metricType string, labels map[string]string) {
	switch metricType {
	case "histogram":
		if upperBound, ok := labels["le"]; ok {
			delete(labels, "le")
			labels["upper_bound"] = canonicalizeNumericLabel(upperBound)
		}
	case "summary":
		if quantile, ok := labels["quantile"]; ok {
			labels["quantile"] = canonicalizeNumericLabel(quantile)
		}
	}
}

func labelContext(labels map[string]string, excludedLabel string) string {
	parts := make([]string, 0, len(labels))
	for label, value := range labels {
		if label == excludedLabel || label == nameLabel {
			continue
		}
		parts = append(parts, label+"="+value)
	}
	sort.Strings(parts)
	return fmt.Sprint(parts)
}
