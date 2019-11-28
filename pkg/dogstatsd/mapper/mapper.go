// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Mapping feature is inspired by https://github.com/prometheus/statsd_exporter

package mapper

import (
	"fmt"
	"regexp"
	"strings"
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
	Name     string           `mapstructure:"name"`
	Prefix   string           `mapstructure:"prefix"`
	Mappings []*MetricMapping `mapstructure:"mappings"`
}

// MetricMapping represent one mapping rule
type MetricMapping struct {
	Match     string            `mapstructure:"match"`
	MatchType string            `mapstructure:"match_type"`
	Name      string            `mapstructure:"name"`
	Tags      map[string]string `mapstructure:"tags"`
	regex     *regexp.Regexp
}

// MapResult represent the outcome of the mapping
type MapResult struct {
	Name    string
	Tags    []string
	Matched bool
}

// NewMetricMapper creates, validates, prepares a new MetricMapper
func NewMetricMapper(profiles []MappingProfile, cacheSize int) (*MetricMapper, error) {
	for profileIndex, profile := range profiles {
		if profile.Name == "" {
			return nil, fmt.Errorf("missing profile name %d", profileIndex)
		}
		if profile.Prefix == "" {
			return nil, fmt.Errorf("missing prefix for profile: %s", profile.Name)
		}
		for i, currentMapping := range profile.Mappings {
			if currentMapping.MatchType == "" {
				currentMapping.MatchType = matchTypeWildcard
			}
			if currentMapping.MatchType != matchTypeWildcard && currentMapping.MatchType != matchTypeRegex {
				return nil, fmt.Errorf("profile: %s, mapping num %d: invalid match type, must be `wildcard` or `regex`", profile.Name, i)
			}
			if currentMapping.Name == "" {
				return nil, fmt.Errorf("profile: %s, mapping num %d: name is required", profile.Name, i)
			}
			if currentMapping.Match == "" {
				return nil, fmt.Errorf("profile: %s, mapping num %d: match is required", profile.Name, i)
			}
			err := currentMapping.prepare()
			if err != nil {
				return nil, err
			}
		}
	}
	cache, err := newMapperCache(cacheSize)
	if err != nil {
		return nil, err
	}
	return &MetricMapper{Profiles: profiles, cache: cache}, nil
}

// prepare compiles the match patterns into regexes
func (m *MetricMapping) prepare() error {
	metricRe := m.Match
	if m.MatchType == matchTypeWildcard {
		if !allowedWildcardMatchPattern.MatchString(m.Match) {
			return fmt.Errorf("invalid wildcard match pattern `%s`, it does not match allowed match regex `%s`", m.Match, allowedWildcardMatchPattern)
		}
		if strings.Contains(m.Match, "**") {
			return fmt.Errorf("invalid wildcard match pattern `%s`, it should not contain consecutive `*`", m.Match)
		}
		metricRe = strings.Replace(metricRe, ".", "\\.", -1)
		metricRe = strings.Replace(metricRe, "*", "([^.]*)", -1)
	}
	regex, err := regexp.Compile("^" + metricRe + "$")
	if err != nil {
		return fmt.Errorf("invalid match `%s`. cannot compile regex: %v", m.Match, err)
	}
	m.regex = regex
	return nil
}

// Map returns a MapResult
func (m *MetricMapper) Map(metricName string) (*MapResult, bool) {
	for _, profile := range m.Profiles {
		if !strings.HasPrefix(metricName, profile.Prefix) && profile.Prefix != "*" {
			continue
		}
		result, cached := m.cache.get(metricName)
		if cached {
			return result, true
		}
		for _, mapping := range profile.Mappings {
			matches := mapping.regex.FindStringSubmatchIndex(metricName)
			if len(matches) == 0 {
				continue
			}

			name := string(mapping.regex.ExpandString(
				[]byte{},
				mapping.Name,
				metricName,
				matches,
			))

			var tags []string
			for tagKey, tagValueExpr := range mapping.Tags {
				tagValue := string(mapping.regex.ExpandString([]byte{}, tagValueExpr, metricName, matches))
				tags = append(tags, tagKey+":"+tagValue)
			}

			mapResult := &MapResult{Name: name, Matched: true, Tags: tags}
			m.cache.add(metricName, mapResult)
			return mapResult, true
		}
		mapResult := &MapResult{Matched: false}
		m.cache.add(metricName, mapResult)
		return mapResult, true
	}
	return nil, false
}
