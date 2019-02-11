// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompileShouldSucceedWithValidRules(t *testing.T) {
	rules := ProcessingRules{[]*ProcessingRule{{Pattern: "[[:alnum:]]{5}", Type: IncludeAtMatch}}}
	err := rules.Compile()
	assert.Nil(t, err)
	assert.NotNil(t, rules.Rules[0].Regex)
	assert.True(t, rules.Rules[0].Regex.MatchString("abcde"))
}

func TestCompileShouldFailWithInvalidRules(t *testing.T) {
	invalidRules := []*ProcessingRule{
		{Type: IncludeAtMatch, Pattern: "(?=abf)"},
	}

	for _, rule := range invalidRules {
		rules := ProcessingRules{[]*ProcessingRule{rule}}
		err := rules.Compile()
		assert.NotNil(t, err)
		assert.Nil(t, rule.Regex)
	}
}
