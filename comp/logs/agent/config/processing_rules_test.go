// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// ---- LocalProcessingOnly / RemoteProcessingOnly validation ----

func TestValidateLocalProcessingOnlyRequiresMatchBlock(t *testing.T) {
	rule := &ProcessingRule{Name: "no-match", Type: LocalProcessingOnly}
	err := ValidateProcessingRules([]*ProcessingRule{rule})
	assert.Error(t, err)
}

func TestValidateLocalProcessingOnlyRequiresNonEmptyServices(t *testing.T) {
	rule := &ProcessingRule{
		Name:  "empty-services",
		Type:  LocalProcessingOnly,
		Match: &RoutingMatch{Services: []string{}},
	}
	err := ValidateProcessingRules([]*ProcessingRule{rule})
	assert.Error(t, err)
}

func TestValidateLocalProcessingOnlyAcceptsValidRule(t *testing.T) {
	rule := &ProcessingRule{
		Name:  "valid",
		Type:  LocalProcessingOnly,
		Match: &RoutingMatch{Services: []string{"web-*", "api-*"}},
	}
	err := ValidateProcessingRules([]*ProcessingRule{rule})
	assert.NoError(t, err)
}

func TestValidateLocalProcessingOnlyRejectsInvalidGlob(t *testing.T) {
	rule := &ProcessingRule{
		Name:  "bad-glob",
		Type:  LocalProcessingOnly,
		Match: &RoutingMatch{Services: []string{"[invalid"}},
	}
	err := ValidateProcessingRules([]*ProcessingRule{rule})
	assert.Error(t, err)
}

// ---- CompileProcessingRules for routing rules ----

func TestCompileLocalProcessingOnlyBuildsCompiledMatch(t *testing.T) {
	rule := &ProcessingRule{
		Name:  "compile-test",
		Type:  LocalProcessingOnly,
		Match: &RoutingMatch{Services: []string{"web-*"}},
	}
	err := CompileProcessingRules([]*ProcessingRule{rule})
	require.NoError(t, err)
	require.NotNil(t, rule.CompiledMatch)
	assert.Equal(t, []string{"web-*"}, rule.CompiledMatch.ServiceGlobs)
}

func TestCompileLocalProcessingOnlyWithNilMatchIsNoop(t *testing.T) {
	rule := &ProcessingRule{Name: "nil-match", Type: LocalProcessingOnly, Match: nil}
	err := CompileProcessingRules([]*ProcessingRule{rule})
	assert.NoError(t, err)
	assert.Nil(t, rule.CompiledMatch)
}

// ---- CompiledRoutingMatch.Matches ----

func TestMatchesServiceGlobOR(t *testing.T) {
	m := &CompiledRoutingMatch{ServiceGlobs: []string{"web-*", "api-*"}}
	assert.True(t, m.Matches("web-frontend"))
	assert.True(t, m.Matches("api-gateway"))
	assert.False(t, m.Matches("db-primary"))
}

func TestMatchesEmptyServiceGlobMatchesAll(t *testing.T) {
	m := &CompiledRoutingMatch{ServiceGlobs: []string{}}
	assert.True(t, m.Matches("anything"))
	assert.True(t, m.Matches(""))
}

func TestMatchesWildcardServiceMatchesEmpty(t *testing.T) {
	m := &CompiledRoutingMatch{ServiceGlobs: []string{"*"}}
	assert.True(t, m.Matches(""))
	assert.True(t, m.Matches("any-service"))
}

func TestMatchesNoOriginEmptyServiceDoesNotMatchPrefixGlob(t *testing.T) {
	m := &CompiledRoutingMatch{ServiceGlobs: []string{"web-*"}}
	// Empty service string does not match "web-*"
	assert.False(t, m.Matches(""))
}
