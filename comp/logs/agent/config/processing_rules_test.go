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

func TestValidateRemapAttributeToSource(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapAttributeToSource,
			Name: "remap_test",
			Mappings: []*SourceMappingEntry{
				{Attribute: "siem.device_vendor", Value: "Security", RemapSourceTo: "arcsight"},
				{Attribute: "siem.device_product", Value: "palo alto", RemapSourceTo: "pan"},
			},
		}}
		assert.NoError(t, ValidateProcessingRules(rules))
	})

	t.Run("missing mappings", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapAttributeToSource,
			Name: "remap_test",
		}}
		assert.ErrorContains(t, ValidateProcessingRules(rules), "no mappings provided")
	})

	t.Run("empty attribute", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapAttributeToSource,
			Name: "remap_test",
			Mappings: []*SourceMappingEntry{
				{Attribute: "", Value: "x", RemapSourceTo: "y"},
			},
		}}
		assert.ErrorContains(t, ValidateProcessingRules(rules), "empty attribute")
	})

	t.Run("empty value", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapAttributeToSource,
			Name: "remap_test",
			Mappings: []*SourceMappingEntry{
				{Attribute: "a", Value: "", RemapSourceTo: "y"},
			},
		}}
		assert.ErrorContains(t, ValidateProcessingRules(rules), "empty value")
	})

	t.Run("empty remap_source_to", func(t *testing.T) {
		rules := []*ProcessingRule{{
			Type: RemapAttributeToSource,
			Name: "remap_test",
			Mappings: []*SourceMappingEntry{
				{Attribute: "a", Value: "b", RemapSourceTo: ""},
			},
		}}
		assert.ErrorContains(t, ValidateProcessingRules(rules), "empty remap_source_to")
	})
}

func TestCompileSkipsRemapAttributeToSource(t *testing.T) {
	rules := []*ProcessingRule{{
		Type: RemapAttributeToSource,
		Name: "remap_test",
		Mappings: []*SourceMappingEntry{
			{Attribute: "siem.device_vendor", Value: "Security", RemapSourceTo: "arcsight"},
		},
	}}
	err := CompileProcessingRules(rules)
	assert.NoError(t, err)
	assert.Nil(t, rules[0].Regex)
}
