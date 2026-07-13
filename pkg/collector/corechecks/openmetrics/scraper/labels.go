// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"strconv"
	"strings"
)

// LabelProcessor handles converting Prometheus metric labels into Datadog tags,
// with support for renaming, filtering, and hostname extraction.
type LabelProcessor struct {
	renameLabels   map[string]string
	excludeLabels  map[string]struct{}
	includeLabels  map[string]struct{}
	hostnameLabel  string
	hostnameFormat string
}

// NewLabelProcessor creates a LabelProcessor from config.
func NewLabelProcessor(cfg *Config) *LabelProcessor {
	lp := &LabelProcessor{
		renameLabels:   cfg.RenameLabels,
		hostnameLabel:  cfg.HostnameLabel,
		hostnameFormat: cfg.HostnameFormat,
	}

	if len(cfg.ExcludeLabels) > 0 {
		lp.excludeLabels = make(map[string]struct{}, len(cfg.ExcludeLabels))
		for _, l := range cfg.ExcludeLabels {
			lp.excludeLabels[l] = struct{}{}
		}
	}

	if len(cfg.IncludeLabels) > 0 {
		lp.includeLabels = make(map[string]struct{}, len(cfg.IncludeLabels))
		for _, l := range cfg.IncludeLabels {
			lp.includeLabels[l] = struct{}{}
		}
	}

	return lp
}

// ProcessLabels converts a sample's labels into Datadog tags and extracts a hostname if configured.
// metricType should be "HISTOGRAM", "SUMMARY", or other — used for label normalization.
func (lp *LabelProcessor) ProcessLabels(sampleLabels map[string]string, metricType string) (tags []string, hostname string) {
	tags = make([]string, 0, len(sampleLabels))

	for label, value := range sampleLabels {
		// Always exclude the __name__ label.
		if label == "__name__" {
			continue
		}

		// Hostname extraction: if this label matches hostname_label, extract the
		// hostname value and do not include it as a tag.
		if lp.hostnameLabel != "" && label == lp.hostnameLabel {
			hostname = value
			if lp.hostnameFormat != "" {
				hostname = strings.Replace(lp.hostnameFormat, "<HOSTNAME>", hostname, 1)
			}
			continue
		}

		// Include filter: if include_labels is configured, only keep labels present
		// in the set. This check uses the original label name before rename.
		if lp.includeLabels != nil {
			if _, ok := lp.includeLabels[label]; !ok {
				continue
			}
		}

		// Exclude filter: skip labels present in the exclude set.
		if lp.excludeLabels != nil {
			if _, ok := lp.excludeLabels[label]; ok {
				continue
			}
		}

		// Histogram label normalization: rename "le" → "upper_bound".
		if metricType == "HISTOGRAM" && label == "le" {
			label = "upper_bound"
		}

		// Summary label normalization: canonicalize quantile float values.
		if metricType == "SUMMARY" && label == "quantile" {
			value = canonicalizeQuantile(value)
		}

		// Apply rename_labels mapping.
		if lp.renameLabels != nil {
			if newName, ok := lp.renameLabels[label]; ok {
				label = newName
			}
		}

		tags = append(tags, label+":"+value)
	}

	return tags, hostname
}

// canonicalizeQuantile parses a quantile string as a float64 and formats it
// back to a clean representation (e.g., "0.50000" → "0.5").
func canonicalizeQuantile(v string) string {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return v
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
