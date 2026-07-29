// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheus

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ProcessConfig holds the label/tag processing configuration passed from Python.
type ProcessConfig struct {
	// ExcludeLabels is the set of label names to drop from tags.
	ExcludeLabels []string `json:"exclude_labels"`
	// IncludeLabels if non-empty, only these label names become tags.
	IncludeLabels []string `json:"include_labels"`
	// RenameLabels maps original label names to new tag names.
	RenameLabels map[string]string `json:"rename_labels"`
	// ExcludeMetrics is the set of exact metric names to skip.
	ExcludeMetrics []string `json:"exclude_metrics"`
	// ExcludeMetricsPatterns is a list of regex patterns for metric exclusion.
	ExcludeMetricsPatterns []string `json:"exclude_metrics_patterns"`
	// ExcludeMetricsByLabels maps label names to lists of regex patterns;
	// samples matching any pattern are skipped. An empty list means skip any value.
	ExcludeMetricsByLabels map[string][]string `json:"exclude_metrics_by_labels"`
	// RawMetricPrefix is stripped from the beginning of all metric names.
	RawMetricPrefix string `json:"raw_metric_prefix"`
	// HostnameLabel is the label name whose value becomes the hostname.
	HostnameLabel string `json:"hostname_label"`
	// HostnameFormat is a template with <HOSTNAME> placeholder.
	HostnameFormat string `json:"hostname_format"`
	// StaticTags are appended to every sample's tags.
	StaticTags []string `json:"static_tags"`
	// ShareLabels configures label propagation from source metrics.
	ShareLabels map[string]ShareLabelConfig `json:"share_labels"`
}

// ShareLabelConfig configures how labels from one metric are shared to others.
type ShareLabelConfig struct {
	// Match is the set of label names used for matching (join keys).
	Match []string `json:"match"`
	// Labels is the set of label names to propagate. Empty means all.
	Labels []string `json:"labels"`
	// Values restricts to samples whose value is in this set.
	Values []float64 `json:"values"`
}

// ProcessedSample is a single metric sample with pre-built tags.
type ProcessedSample struct {
	SampleName string   `json:"sample_name"`
	Value      float64  `json:"value"`
	Tags       []string `json:"tags"`
	Hostname   string   `json:"hostname,omitempty"`
}

// ProcessedMetricFamily is a metric family with processed samples.
type ProcessedMetricFamily struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Samples []ProcessedSample `json:"samples"`
}

// compiledConfig holds pre-compiled versions of the processing config.
type compiledConfig struct {
	excludeLabels          map[string]struct{}
	includeLabels          map[string]struct{}
	renameLabels           map[string]string
	excludeMetrics         map[string]struct{}
	excludeMetricsPattern  *regexp.Regexp
	excludeMetricsByLabels map[string]*regexp.Regexp // nil regexp means "any value"
	rawMetricPrefix        string
	hostnameLabel          string
	hostnameFormat         string
	staticTags             []string
	shareLabels            map[string]compiledShareLabel
}

type compiledShareLabel struct {
	match     map[string]struct{}
	labels    map[string]struct{}
	values    map[float64]struct{}
	allLabels bool
	anyValue  bool
}

func compileConfig(cfg *ProcessConfig) (*compiledConfig, error) {
	cc := &compiledConfig{
		excludeLabels:   make(map[string]struct{}, len(cfg.ExcludeLabels)),
		includeLabels:   make(map[string]struct{}, len(cfg.IncludeLabels)),
		renameLabels:    cfg.RenameLabels,
		excludeMetrics:  make(map[string]struct{}, len(cfg.ExcludeMetrics)),
		rawMetricPrefix: cfg.RawMetricPrefix,
		hostnameLabel:   cfg.HostnameLabel,
		hostnameFormat:  cfg.HostnameFormat,
		staticTags:      cfg.StaticTags,
		shareLabels:     make(map[string]compiledShareLabel, len(cfg.ShareLabels)),
	}
	if cc.renameLabels == nil {
		cc.renameLabels = map[string]string{}
	}
	if cc.staticTags == nil {
		cc.staticTags = []string{}
	}

	for _, l := range cfg.ExcludeLabels {
		cc.excludeLabels[l] = struct{}{}
	}
	for _, l := range cfg.IncludeLabels {
		cc.includeLabels[l] = struct{}{}
	}
	for _, m := range cfg.ExcludeMetrics {
		cc.excludeMetrics[m] = struct{}{}
	}

	if len(cfg.ExcludeMetricsPatterns) > 0 {
		combined := strings.Join(cfg.ExcludeMetricsPatterns, "|")
		p, err := regexp.Compile(combined)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude_metrics_patterns: %w", err)
		}
		cc.excludeMetricsPattern = p
	}

	cc.excludeMetricsByLabels = make(map[string]*regexp.Regexp, len(cfg.ExcludeMetricsByLabels))
	for label, patterns := range cfg.ExcludeMetricsByLabels {
		if len(patterns) == 0 {
			// empty list means "any value"
			cc.excludeMetricsByLabels[label] = nil
			continue
		}
		combined := strings.Join(patterns, "|")
		p, err := regexp.Compile(combined)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude_metrics_by_labels pattern for %q: %w", label, err)
		}
		cc.excludeMetricsByLabels[label] = p
	}

	for name, slCfg := range cfg.ShareLabels {
		csl := compiledShareLabel{
			match:     make(map[string]struct{}, len(slCfg.Match)),
			labels:    make(map[string]struct{}, len(slCfg.Labels)),
			values:    make(map[float64]struct{}, len(slCfg.Values)),
			allLabels: len(slCfg.Labels) == 0,
			anyValue:  len(slCfg.Values) == 0,
		}
		for _, m := range slCfg.Match {
			csl.match[m] = struct{}{}
		}
		for _, l := range slCfg.Labels {
			csl.labels[l] = struct{}{}
		}
		for _, v := range slCfg.Values {
			csl.values[v] = struct{}{}
		}
		cc.shareLabels[name] = csl
	}

	return cc, nil
}

// ProcessMetrics parses prometheus-formatted metrics and applies label/tag processing.
func ProcessMetrics(data []byte, contentType string, cfg *ProcessConfig) ([]ProcessedMetricFamily, error) {
	families, err := ParseMetricsWithFilter(data, nil, contentType)
	if err != nil {
		return nil, err
	}

	cc, err := compileConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Phase 1: strip prefix and collect shared labels
	for i := range families {
		if cc.rawMetricPrefix != "" && strings.HasPrefix(families[i].Name, cc.rawMetricPrefix) {
			families[i].Name = families[i].Name[len(cc.rawMetricPrefix):]
		}
	}

	sharedState := collectSharedLabels(families, cc)

	// Phase 2: process each family
	result := make([]ProcessedMetricFamily, 0, len(families))
	for _, fam := range families {
		// Check metric exclusion
		if _, excluded := cc.excludeMetrics[fam.Name]; excluded {
			continue
		}
		if cc.excludeMetricsPattern != nil && cc.excludeMetricsPattern.MatchString(fam.Name) {
			continue
		}

		processed := processFamily(fam, cc, sharedState)
		if len(processed.Samples) > 0 {
			result = append(result, processed)
		}
	}

	return result, nil
}

// ProcessMetricsToJSON processes metrics and returns the result as JSON.
func ProcessMetricsToJSON(data []byte, contentType string, configJSON string) (string, error) {
	var cfg ProcessConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", fmt.Errorf("invalid config: %w", err)
	}

	families, err := ProcessMetrics(data, contentType, &cfg)
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(families)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// sharedLabelState holds collected labels from share_labels source metrics.
type sharedLabelState struct {
	// unconditional labels applied to all samples (no match key required)
	unconditional map[string]string
	// conditional labels: each entry is (matchSet, sharedLabels)
	conditional []conditionalLabels
}

type conditionalLabels struct {
	matchSet     map[labelPair]struct{}
	sharedLabels map[string]string
}

type labelPair struct {
	name  string
	value string
}

func collectSharedLabels(families []MetricFamily, cc *compiledConfig) *sharedLabelState {
	state := &sharedLabelState{
		unconditional: make(map[string]string),
	}

	if len(cc.shareLabels) == 0 {
		return state
	}

	for _, fam := range families {
		slCfg, ok := cc.shareLabels[fam.Name]
		if !ok {
			continue
		}

		for _, sample := range fam.Samples {
			// Check value restriction
			if !slCfg.anyValue {
				if _, allowed := slCfg.values[sample.Value]; !allowed {
					continue
				}
			}

			if len(slCfg.match) > 0 {
				// Conditional: collect match keys and shared labels
				matchSet := make(map[labelPair]struct{})
				shared := make(map[string]string)

				for labelName, labelValue := range sample.Metric {
					if _, isMatch := slCfg.match[labelName]; isMatch {
						matchSet[labelPair{labelName, labelValue}] = struct{}{}
					}
					if slCfg.allLabels || isInSet(labelName, slCfg.labels) {
						shared[labelName] = labelValue
					}
				}
				state.conditional = append(state.conditional, conditionalLabels{
					matchSet:     matchSet,
					sharedLabels: shared,
				})
			} else {
				// Unconditional: apply to all samples
				for labelName, labelValue := range sample.Metric {
					if slCfg.allLabels || isInSet(labelName, slCfg.labels) {
						state.unconditional[labelName] = labelValue
					}
				}
			}
		}
	}

	return state
}

func isInSet(key string, set map[string]struct{}) bool {
	_, ok := set[key]
	return ok
}

func processFamily(fam MetricFamily, cc *compiledConfig, shared *sharedLabelState) ProcessedMetricFamily {
	result := ProcessedMetricFamily{
		Name:    fam.Name,
		Type:    fam.Type,
		Samples: make([]ProcessedSample, 0, len(fam.Samples)),
	}

	famType := strings.ToLower(fam.Type)

	for _, sample := range fam.Samples {
		// Skip NaN/Inf
		if math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0) {
			continue
		}

		// Build effective labels: start with sample labels, apply shared
		labels := make(map[string]string, len(sample.Metric))
		for k, v := range sample.Metric {
			labels[k] = v
		}

		// Apply shared labels
		applySharedLabels(labels, shared)

		// Normalize histogram/summary labels
		normalizeLabels(labels, famType)

		// Check exclude_metrics_by_labels
		if shouldExcludeByLabels(labels, cc) {
			continue
		}

		// Build tags and extract hostname
		tags, hostname := buildTags(labels, cc)

		// Determine sample name from the __name__ label or metric name
		sampleName := labels["__name__"]
		if sampleName == "" {
			sampleName = fam.Name
		}

		result.Samples = append(result.Samples, ProcessedSample{
			SampleName: sampleName,
			Value:      sample.Value,
			Tags:       tags,
			Hostname:   hostname,
		})
	}

	return result
}

func applySharedLabels(labels map[string]string, shared *sharedLabelState) {
	// Apply unconditional labels first
	for k, v := range shared.unconditional {
		if _, exists := labels[k]; !exists {
			labels[k] = v
		}
	}

	// Apply conditional labels
	for _, cond := range shared.conditional {
		if matchesLabelSet(labels, cond.matchSet) {
			for k, v := range cond.sharedLabels {
				if _, exists := labels[k]; !exists {
					labels[k] = v
				}
			}
		}
	}
}

func matchesLabelSet(labels map[string]string, matchSet map[labelPair]struct{}) bool {
	for pair := range matchSet {
		if labels[pair.name] != pair.value {
			return false
		}
	}
	return true
}

func normalizeLabels(labels map[string]string, metricType string) {
	switch metricType {
	case "histogram":
		// Rename le → upper_bound with canonical numeric value
		if le, ok := labels["le"]; ok {
			delete(labels, "le")
			labels["upper_bound"] = canonicalizeNumericLabel(le)
		}
	case "summary":
		// Canonicalize quantile value
		if q, ok := labels["quantile"]; ok {
			labels["quantile"] = canonicalizeNumericLabel(q)
		}
	}
}

// canonicalizeNumericLabel converts a numeric label to its canonical string form.
// This matches the Python canonicalize_numeric_label: float(label) or 0.
func canonicalizeNumericLabel(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	// Prevent -0.0
	if f == 0 {
		f = 0
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func shouldExcludeByLabels(labels map[string]string, cc *compiledConfig) bool {
	for labelName, pattern := range cc.excludeMetricsByLabels {
		labelValue, exists := labels[labelName]
		if !exists {
			continue
		}
		if pattern == nil {
			// nil means "any value matches"
			return true
		}
		if pattern.MatchString(labelValue) {
			return true
		}
	}
	return false
}

func buildTags(labels map[string]string, cc *compiledConfig) ([]string, string) {
	hasIncludeFilter := len(cc.includeLabels) > 0
	hostname := ""

	// Pre-allocate tags: labels + static tags
	tags := make([]string, 0, len(labels)+len(cc.staticTags))

	for labelName, labelValue := range labels {
		// Skip __name__ — it's metadata, not a tag
		if labelName == "__name__" {
			continue
		}

		// Check exclude
		if _, excluded := cc.excludeLabels[labelName]; excluded {
			continue
		}

		// Check include filter
		if hasIncludeFilter {
			if _, included := cc.includeLabels[labelName]; !included {
				continue
			}
		}

		// Apply rename
		tagName := labelName
		if renamed, ok := cc.renameLabels[labelName]; ok {
			tagName = renamed
		}

		tags = append(tags, tagName+":"+labelValue)
	}

	// Append static tags
	tags = append(tags, cc.staticTags...)

	// Extract hostname
	if cc.hostnameLabel != "" {
		if h, ok := labels[cc.hostnameLabel]; ok {
			hostname = h
			if cc.hostnameFormat != "" {
				hostname = strings.Replace(cc.hostnameFormat, "<HOSTNAME>", hostname, 1)
			}
		}
	}

	return tags, hostname
}
