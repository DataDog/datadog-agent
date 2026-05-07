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
