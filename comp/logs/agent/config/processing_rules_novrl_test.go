// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !vrl && !windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestVRLRulesFailClearlyWithoutBuildTag documents that, in a default build
// (no `vrl` build tag), VRL processing rules fail loudly at config-validate
// and compile time rather than silently becoming no-ops.
func TestVRLRulesFailClearlyWithoutBuildTag(t *testing.T) {
	for _, ruleType := range []string{ExcludeAtVRLMatch, IncludeAtVRLMatch, MaskVRLTransform} {
		t.Run(ruleType, func(t *testing.T) {
			rules := []*ProcessingRule{{Type: ruleType, Name: "vrl_test", Pattern: `.status == "debug"`}}
			assert.ErrorContains(t, ValidateProcessingRules(rules), "vrl' build tag")
			assert.ErrorContains(t, CompileProcessingRules(rules), "vrl' build tag")
			assert.Nil(t, rules[0].VRLFilter)
			assert.Nil(t, rules[0].VRLTransform)
		})
	}
}
