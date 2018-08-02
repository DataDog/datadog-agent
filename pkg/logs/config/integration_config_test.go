// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {

}

func TestCompile(t *testing.T) {
	var config *LogsConfig
	var rules []LogsProcessingRule
	var err error

	rules = []LogsProcessingRule{{Pattern: "(?=abf)", Type: IncludeAtMatch}}
	config = &LogsConfig{ProcessingRules: rules}
	err = Compile(config)
	assert.NotNil(t, err)
	assert.Nil(t, rules[0].Reg)

	rules = []LogsProcessingRule{{Pattern: "[[:alnum:]]{5}", Type: IncludeAtMatch}}
	config = &LogsConfig{ProcessingRules: rules}
	err = Compile(config)
	assert.Nil(t, err)
	assert.NotNil(t, rules[0].Reg)
	assert.True(t, rules[0].Reg.MatchString("abcde"))

	rules = []LogsProcessingRule{{Pattern: "[[:alnum:]]{5}"}}
	config = &LogsConfig{ProcessingRules: rules}
	err = Compile(config)
	assert.NotNil(t, err)
	assert.Nil(t, rules[0].Reg)

	rules = []LogsProcessingRule{{Pattern: "", Type: IncludeAtMatch}}
	config = &LogsConfig{ProcessingRules: rules}
	err = Compile(config)
	assert.NotNil(t, err)
	assert.Nil(t, rules[0].Reg)
}
