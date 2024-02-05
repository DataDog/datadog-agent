// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Mapping feature is inspired by https://github.com/prometheus/statsd_exporter

//nolint:revive // TODO(AML) Fix revive linter
package mapper

import (
	"fmt"
	"regexp"
	"strings"

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
	profiles := make([]MappingProfile, 0, len(configProfiles))
	for profileIndex, configProfile := range configProfiles {
		if configProfile.Name == "" {
			return nil, fmt.Errorf("missing profile name %d", profileIndex)
		}
		if configProfile.Prefix == "" {
			return nil, fmt.Errorf("missing prefix for profile: %s", configProfile.Name)
		}
		profile := MappingProfile{
			Name:     configProfile.Name,
			Prefix:   configProfile.Prefix,
			Mappings: make([]*MetricMapping, 0, len(configProfile.Mappings)),
		}
		for i, currentMapping := range configProfile.Mappings {
			matchType := currentMapping.MatchType
			if matchType == "" {
				matchType = matchTypeWildcard
			}
			if matchType != matchTypeWildcard && matchType != matchTypeRegex {
				return nil, fmt.Errorf("profile: %s, mapping num %d: invalid match type, must be `wildcard` or `regex`", profile.Name, i)
			}
			if currentMapping.Name == "" {
				return nil, fmt.Errorf("profile: %s, mapping num %d: name is required", profile.Name, i)
			}
			if currentMapping.Match == "" {
				return nil, fmt.Errorf("profile: %s, mapping num %d: match is required", profile.Name, i)
			}
			regex, err := buildRegex(currentMapping.Match, matchType)
			if err != nil {
				return nil, err
			}
			profile.Mappings = append(profile.Mappings, &MetricMapping{name: currentMapping.Name, tags: currentMapping.Tags, regex: regex})
		}
		profiles = append(profiles, profile)
	}
	cache, err := newMapperCache(cacheSize)
	if err != nil {
		return nil, err
	}
	return &MetricMapper{Profiles: profiles, cache: cache}, nil
}

func buildRegex(matchRe string, matchType string) (*regexp.Regexp, error) {
	if matchType == matchTypeWildcard {
		if !allowedWildcardMatchPattern.MatchString(matchRe) {
			return nil, fmt.Errorf("invalid wildcard match pattern `%s`, it does not match allowed match regex `%s`", matchRe, allowedWildcardMatchPattern)
		}
		if strings.Contains(matchRe, "**") {
			return nil, fmt.Errorf("invalid wildcard match pattern `%s`, it should not contain consecutive `*`", matchRe)
		}
		matchRe = strings.Replace(matchRe, ".", "\\.", -1)
		matchRe = strings.Replace(matchRe, "*", "([^.]*)", -1)
	}
	regex, err := regexp.Compile("^" + matchRe + "$")
	if err != nil {
		return nil, fmt.Errorf("invalid match `%s`. cannot compile regex: %v", matchRe, err)
	}
	return regex, nil
}

// Map returns a MapResult
func (m *MetricMapper) Map(metricName string) *MapResult {
	for _, profile := range m.Profiles {
		if !strings.HasPrefix(metricName, profile.Prefix) && profile.Prefix != "*" {
			continue
		}
		result, cached := m.cache.get(metricName)
		if cached {
			if result.matched {
				return result
			}
			return nil
		}
		for _, mapping := range profile.Mappings {
			matches := mapping.regex.FindStringSubmatchIndex(metricName)
			if len(matches) == 0 {
				continue
			}

			name := string(mapping.regex.ExpandString(
				[]byte{},
				mapping.name,
				metricName,
				matches,
			))

			tags := make([]string, 0, len(mapping.tags))
			for tagKey, tagValueExpr := range mapping.tags {
				tagValue := string(mapping.regex.ExpandString([]byte{}, tagValueExpr, metricName, matches))
				tags = append(tags, tagKey+":"+tagValue)
			}

			mapResult := &MapResult{Name: name, matched: true, Tags: tags}
			m.cache.add(metricName, mapResult)
			return mapResult
		}
		mapResult := &MapResult{matched: false}
		m.cache.add(metricName, mapResult)
		return nil
	}
	return nil
}
