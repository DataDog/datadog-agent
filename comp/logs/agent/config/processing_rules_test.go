// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompileShouldSucceedWithValidRules(t *testing.T) {
	rules := []*ProcessingRule{{Pattern: "[[:alnum:]]{5}", Type: IncludeAtMatch}}
	err := CompileProcessingRules(rules)
	assert.Nil(t, err)
	assert.NotNil(t, rules[0].Regex)
	assert.True(t, rules[0].Regex.MatchString("abcde"))
}

func TestCompileShouldFailWithInvalidRules(t *testing.T) {
	invalidRules := []*ProcessingRule{
		{Type: IncludeAtMatch, Pattern: "(?=abf)"},
	}

	for _, rule := range invalidRules {
		rules := []*ProcessingRule{rule}
		err := CompileProcessingRules(rules)
		assert.NotNil(t, err)
		assert.Nil(t, rule.Regex)
	}
}

func TestValidateRoutingRules(t *testing.T) {
	t.Run("EdgeOnly_valid", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Name: "test",
			Type: EdgeOnly,
			Match: &RoutingMatch{
				Services:   []string{"web-*"},
				Severities: []string{"error"},
			},
		}}
		assert.NoError(t, ValidateProcessingRules(rules))
	})

	t.Run("ExcludeFromObserver_valid", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Name: "test",
			Type: ExcludeFromObserver,
			Match: &RoutingMatch{
				Sources: []string{"nginx", "apache-*"},
			},
		}}
		assert.NoError(t, ValidateProcessingRules(rules))
	})

	t.Run("missing_match_block", func(t *testing.T) {
		rules := []*ProcessingRule{{Name: "test", Type: EdgeOnly}}
		assert.Error(t, ValidateProcessingRules(rules))
	})

	t.Run("invalid_severity", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Name:  "test",
			Type:  EdgeOnly,
			Match: &RoutingMatch{Severities: []string{"verbose"}},
		}}
		assert.Error(t, ValidateProcessingRules(rules))
	})

	t.Run("invalid_service_glob", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Name:  "test",
			Type:  EdgeOnly,
			Match: &RoutingMatch{Services: []string{"[invalid"}},
		}}
		assert.Error(t, ValidateProcessingRules(rules))
	})
}

func TestCompileRoutingRules(t *testing.T) {
	rules := []*ProcessingRule{{
		Name: "test",
		Type: EdgeOnly,
		Match: &RoutingMatch{
			Services:   []string{"web-*", "api"},
			Sources:    []string{"nginx"},
			Severities: []string{"Error", "warn"},
		},
	}}
	assert.NoError(t, CompileProcessingRules(rules))
	assert.NotNil(t, rules[0].CompiledMatch)
}

func TestCompiledRoutingMatchMatches(t *testing.T) {
	compile := func(m *RoutingMatch) *CompiledRoutingMatch {
		c := compileRoutingMatch(m)
		return c
	}

	tests := []struct {
		name    string
		match   *RoutingMatch
		service string
		source  string
		status  string
		want    bool
	}{
		{
			name:    "all_criteria_match",
			match:   &RoutingMatch{Services: []string{"web-*"}, Sources: []string{"nginx"}, Severities: []string{"error"}},
			service: "web-app", source: "nginx", status: "error",
			want: true,
		},
		{
			name:    "service_no_match",
			match:   &RoutingMatch{Services: []string{"web-*"}},
			service: "db", source: "", status: "",
			want: false,
		},
		{
			name:    "source_no_match",
			match:   &RoutingMatch{Sources: []string{"nginx"}},
			service: "", source: "apache", status: "",
			want: false,
		},
		{
			name:    "severity_no_match",
			match:   &RoutingMatch{Severities: []string{"error"}},
			service: "", source: "", status: "info",
			want: false,
		},
		{
			name:    "empty_criteria_matches_all",
			match:   &RoutingMatch{},
			service: "any", source: "any", status: "any",
			want: true,
		},
		{
			name:    "wildcard_service_matches_empty",
			match:   &RoutingMatch{Services: []string{"*"}},
			service: "", source: "", status: "",
			want: true,
		},
		{
			name:    "prefix_glob_no_match_empty_service",
			match:   &RoutingMatch{Services: []string{"web-*"}},
			service: "", source: "", status: "",
			want: false,
		},
		{
			name:    "severity_warning_normalized",
			match:   &RoutingMatch{Severities: []string{"warn"}},
			service: "", source: "", status: "warning",
			want: true,
		},
		{
			name:    "multiple_service_patterns_or",
			match:   &RoutingMatch{Services: []string{"web-*", "api-*"}},
			service: "api-gateway", source: "", status: "",
			want: true,
		},
		{
			name:    "case_insensitive_severity",
			match:   &RoutingMatch{Severities: []string{"Error"}},
			service: "", source: "", status: "error",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := compile(tt.match)
			assert.Equal(t, tt.want, c.Matches(tt.service, tt.source, tt.status))
		})
	}
}
