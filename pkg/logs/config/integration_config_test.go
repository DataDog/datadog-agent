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

func TestValidateShouldSucceedWithValidConfigs(t *testing.T) {
	validConfigs := []*LogsConfig{
		{Type: FileType, Path: "/var/log/foo.log"},
		{Type: TCPType, Port: 1234},
		{Type: UDPType, Port: 5678},
		{Type: DockerType},
		{Type: JournaldType, ProcessingRules: []LogsProcessingRule{{Name: "foo", Type: ExcludeAtMatch, Pattern: ".*"}}},
	}

	for _, config := range validConfigs {
		isValid, err := Validate(config)
		assert.Nil(t, err)
		assert.True(t, isValid)
	}
}

func TestValidateShouldFailWithInvalidConfigs(t *testing.T) {
	invalidConfigs := []*LogsConfig{
		{Type: FileType},
		{Type: TCPType},
		{Type: UDPType},
		{Type: DockerType, ProcessingRules: []LogsProcessingRule{{Name: "foo"}}},
		{Type: DockerType, ProcessingRules: []LogsProcessingRule{{Name: "foo", Type: "bar"}}},
		{Type: DockerType, ProcessingRules: []LogsProcessingRule{{Name: "foo", Type: ExcludeAtMatch}}},
		{Type: DockerType, ProcessingRules: []LogsProcessingRule{{Name: "foo", Pattern: ".*"}}},
		{Type: DockerType, ProcessingRules: []LogsProcessingRule{{Type: ExcludeAtMatch, Pattern: ".*"}}},
		{Type: DockerType, ProcessingRules: []LogsProcessingRule{{Type: ExcludeAtMatch}}},
		{Type: DockerType, ProcessingRules: []LogsProcessingRule{{Pattern: ".*"}}},
	}

	for _, config := range invalidConfigs {
		isValid, err := Validate(config)
		assert.False(t, isValid)
		assert.NotNil(t, err)
	}
}

func TestCompileShouldSucceedWithValidRules(t *testing.T) {
	rules := []LogsProcessingRule{{Pattern: "[[:alnum:]]{5}", Type: IncludeAtMatch}}
	config := &LogsConfig{ProcessingRules: rules}
	err := Compile(config)
	assert.Nil(t, err)
	assert.NotNil(t, rules[0].Reg)
	assert.True(t, rules[0].Reg.MatchString("abcde"))
}

func TestCompileShouldFailWithInvalidRules(t *testing.T) {
	invalidRules := []LogsProcessingRule{
		{Type: IncludeAtMatch, Pattern: "(?=abf)"},
		{Pattern: "[[:alnum:]]{5}"},
	}

	for _, rule := range invalidRules {
		config := &LogsConfig{ProcessingRules: []LogsProcessingRule{rule}}
		err := Compile(config)
		assert.NotNil(t, err)
		assert.Nil(t, rule.Reg)
	}
}
