// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build vrl && !windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVRLFilterRuleCompilesWithBuildTag(t *testing.T) {
	rules := []*ProcessingRule{{Type: ExcludeAtVRLMatch, Name: "vrl_test", Pattern: `.message == "drop me"`}}
	require.NoError(t, ValidateProcessingRules(rules))
	require.NoError(t, CompileProcessingRules(rules))
	require.NotNil(t, rules[0].VRLFilter)
	assert.Nil(t, rules[0].VRLTransform)
	assert.Nil(t, rules[0].Regex)

	matched, err := rules[0].VRLFilter([]byte("drop me"))
	require.NoError(t, err)
	assert.True(t, matched)
}

func TestVRLMaskRuleCompilesWithBuildTag(t *testing.T) {
	rules := []*ProcessingRule{{Type: MaskVRLTransform, Name: "vrl_test", Pattern: `.message = redact!(.message, [r'\d+'])`}}
	require.NoError(t, ValidateProcessingRules(rules))
	require.NoError(t, CompileProcessingRules(rules))
	require.NotNil(t, rules[0].VRLTransform)
	assert.Nil(t, rules[0].VRLFilter)

	out, err := rules[0].VRLTransform([]byte("id 123456"))
	require.NoError(t, err)
	assert.Equal(t, []byte("id [REDACTED]"), out)
}

func TestVRLRuleCompileFailsWithInvalidPattern(t *testing.T) {
	rules := []*ProcessingRule{{Type: ExcludeAtVRLMatch, Name: "vrl_test", Pattern: `.message ==`}}
	assert.Error(t, ValidateProcessingRules(rules))
	assert.Error(t, CompileProcessingRules(rules))
	assert.Nil(t, rules[0].VRLFilter)
}
