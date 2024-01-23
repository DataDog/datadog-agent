// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Mapping feature is inspired by https://github.com/prometheus/statsd_exporter

//nolint:revive // TODO(AML) Fix revive linter
package mapper

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	allowedWildcardMatchPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_*.]+$`)
)

const (
	matchTypeWildcard = "wildcard"
	matchTypeRegex    = "regex"
)

// MetricMapper contains mappings and cache instance
type MetricMapper struct {
	Profiles []MappingProfile
	cache    *mapperCache
}

// MappingProfile represent a group of mappings
type MappingProfile struct {
	Name     string
	Prefix   string
	Mappings []*MetricMapping
}

// MetricMapping represent one mapping rule
type MetricMapping struct {
	name  string
	tags  map[string]string
	regex *regexp.Regexp
}

// MapResult represent the outcome of the mapping
type MapResult struct {
	Name    string
	Tags    []string
	matched bool
}

// NewMetricMapper creates, validates, prepares a new MetricMapper
func NewMetricMapper(configProfiles []config.MappingProfile, cacheSize int) (*MetricMapper, error) {
	panic("not called")
}

func buildRegex(matchRe string, matchType string) (*regexp.Regexp, error) {
	panic("not called")
}

// Map returns a MapResult
func (m *MetricMapper) Map(metricName string) *MapResult {
	panic("not called")
}
