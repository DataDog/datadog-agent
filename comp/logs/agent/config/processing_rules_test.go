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

func TestValidateRemapSource(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapSource,
			Name: "remap_test",
			Matching: []*SourceMatchEntry{
				{Attribute: "siem.device_vendor", Value: "Security", NewSource: "arcsight"},
				{Attribute: "siem.device_product", Value: "palo alto", NewSource: "pan"},
			},
		}}
		assert.NoError(t, ValidateProcessingRules(rules))
	})

	t.Run("missing matching", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapSource,
			Name: "remap_test",
		}}
		assert.ErrorContains(t, ValidateProcessingRules(rules), "no matching entries provided")
	})

	t.Run("empty attribute", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapSource,
			Name: "remap_test",
			Matching: []*SourceMatchEntry{
				{Attribute: "", Value: "x", NewSource: "y"},
			},
		}}
		assert.ErrorContains(t, ValidateProcessingRules(rules), "empty attribute")
	})

	t.Run("empty value", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapSource,
			Name: "remap_test",
			Matching: []*SourceMatchEntry{
				{Attribute: "a", Value: "", NewSource: "y"},
			},
		}}
		assert.ErrorContains(t, ValidateProcessingRules(rules), "empty value")
	})

	t.Run("empty new_source", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapSource,
			Name: "remap_test",
			Matching: []*SourceMatchEntry{
				{Attribute: "a", Value: "b", NewSource: ""},
			},
		}}
		assert.ErrorContains(t, ValidateProcessingRules(rules), "empty new_source")
	})
}

func TestCompileSkipsRemapSource(t *testing.T) {
	rules := []*ProcessingRule{{
		Type: RemapSource,
		Name: "remap_test",
		Matching: []*SourceMatchEntry{
			{Attribute: "siem.device_vendor", Value: "Security", NewSource: "arcsight"},
		},
	}}
	err := CompileProcessingRules(rules)
	assert.NoError(t, err)
	assert.Nil(t, rules[0].Regex)
}

func TestValidateGoJQRules(t *testing.T) {
	t.Run("valid exclude", func(t *testing.T) {
		rules := []*ProcessingRule{{Type: ExcludeAtGoJQMatch, Name: "gojq_test", Pattern: `select(.level == "debug")`}}
		assert.NoError(t, ValidateProcessingRules(rules))
	})

	t.Run("valid include", func(t *testing.T) {
		rules := []*ProcessingRule{{Type: IncludeAtGoJQMatch, Name: "gojq_test", Pattern: `select(.level == "error")`}}
		assert.NoError(t, ValidateProcessingRules(rules))
	})

	t.Run("valid mask", func(t *testing.T) {
		rules := []*ProcessingRule{{Type: MaskGoJQTransform, Name: "gojq_test", Pattern: `.message |= gsub("[0-9]+"; "X")`}}
		assert.NoError(t, ValidateProcessingRules(rules))
	})

	t.Run("empty pattern", func(t *testing.T) {
		for _, ruleType := range []string{ExcludeAtGoJQMatch, IncludeAtGoJQMatch, MaskGoJQTransform} {
			rules := []*ProcessingRule{{Type: ruleType, Name: "gojq_test"}}
			assert.ErrorContains(t, ValidateProcessingRules(rules), "no pattern provided")
		}
	})

	t.Run("invalid jq syntax", func(t *testing.T) {
		for _, ruleType := range []string{ExcludeAtGoJQMatch, IncludeAtGoJQMatch, MaskGoJQTransform} {
			rules := []*ProcessingRule{{Type: ruleType, Name: "gojq_test", Pattern: "("}}
			assert.ErrorContains(t, ValidateProcessingRules(rules), "invalid jq pattern")
		}
	})
}

func TestCompileGoJQFilterRules(t *testing.T) {
	rules := []*ProcessingRule{
		{Type: ExcludeAtGoJQMatch, Name: "gojq_exclude", Pattern: `select(.level == "debug")`},
		{Type: IncludeAtGoJQMatch, Name: "gojq_include", Pattern: `select(.level == "error")`},
	}
	require.NoError(t, CompileProcessingRules(rules))
	require.NotNil(t, rules[0].GoJQFilter)
	require.NotNil(t, rules[1].GoJQFilter)
	assert.Nil(t, rules[0].GoJQTransform)

	matched, err := rules[0].GoJQFilter([]byte(`{"level":"debug"}`))
	require.NoError(t, err)
	assert.True(t, matched)

	matched, err = rules[0].GoJQFilter([]byte(`{"level":"error"}`))
	require.NoError(t, err)
	assert.False(t, matched)

	_, err = rules[0].GoJQFilter([]byte(`not json`))
	assert.Error(t, err)
}

func TestCompileGoJQMaskRule(t *testing.T) {
	rules := []*ProcessingRule{{
		Type:    MaskGoJQTransform,
		Name:    "gojq_mask",
		Pattern: `.message |= gsub("(?<num>[0-9]+)"; "[REDACTED-\(.num)]")`,
	}}
	require.NoError(t, CompileProcessingRules(rules))
	require.NotNil(t, rules[0].GoJQTransform)
	assert.Nil(t, rules[0].GoJQFilter)

	out, err := rules[0].GoJQTransform([]byte(`{"message":"user 123456 logged in"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"message":"user [REDACTED-123456] logged in"}`, string(out))

	_, err = rules[0].GoJQTransform([]byte(`not json`))
	assert.Error(t, err)
}
