// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
)

// MetricTagList is for loading the data from the configuration.
type MetricTagList struct {
	Tags   []string `yaml:"tags"`
	Action string   `yaml:"action"`
}

// MetricTagListEntry is for loading the new list-based configuration format.
type MetricTagListEntry struct {
	MetricName string   `mapstructure:"metric_name" yaml:"metric_name" json:"metric_name"`
	Action     string   `mapstructure:"action" yaml:"action" json:"action"`
	Tags       []string `mapstructure:"tags" yaml:"tags" json:"tags"`
}

func NewEmptyTagMatcher() metricname.TagMatcher {
	return metricname.TagMatcher{}
}

// NewTagMatcher creates a new instance of TagMatcher. The function takes
// a list of metric names and tags. Those tags are hashed using murmur3.
// The hashed value is then used to query whether a tag should be removed
// from a given metric.
func NewTagMatcher(metrics map[string]MetricTagList, log log.Component) metricname.TagMatcher {
	hashed := make(map[string]metricname.HashedMetricTagList, len(metrics))
	for k, v := range metrics {
		var exclude bool
		switch v.Action {
		case "include":
			exclude = false
		case "exclude", "":
			exclude = true
		default:
			log.Warnf("`metric_tag_filterlist.%s.action` configuration value %q should be either `include` or `exclude`. Defaulting to `exclude`.", k, v.Action)
			exclude = true
		}
		hashed[k] = metricname.NewHashedMetricTagList(exclude, v.Tags)
	}

	return metricname.NewTagMatcher(hashed)
}
