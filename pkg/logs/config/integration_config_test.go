// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/stretchr/testify/assert"
)

func TestValidateShouldSucceedWithValidConfigs(t *testing.T) {
	validConfigs := []*LogsConfig{
		{Type: types.FileType, Path: "/var/log/foo.log"},
		{Type: types.TCPType, Port: 1234},
		{Type: types.UDPType, Port: 5678},
		{Type: types.DockerType},
		{Type: types.JournaldType, ProcessingRules: []types.ProcessingRule{{Name: "foo", Type: types.ExcludeAtMatch, Pattern: ".*"}}},
	}

	for _, config := range validConfigs {
		err := config.Validate()
		assert.Nil(t, err)
	}
}

func TestValidateShouldFailWithInvalidConfigs(t *testing.T) {
	invalidConfigs := []*LogsConfig{
		{},
		{Type: types.FileType},
		{Type: types.TCPType},
		{Type: types.UDPType},
		{Type: types.DockerType, ProcessingRules: []types.ProcessingRule{{Name: "foo"}}},
		{Type: types.DockerType, ProcessingRules: []types.ProcessingRule{{Name: "foo", Type: "bar"}}},
		{Type: types.DockerType, ProcessingRules: []types.ProcessingRule{{Name: "foo", Type: types.ExcludeAtMatch}}},
		{Type: types.DockerType, ProcessingRules: []types.ProcessingRule{{Name: "foo", Pattern: ".*"}}},
		{Type: types.DockerType, ProcessingRules: []types.ProcessingRule{{Type: types.ExcludeAtMatch, Pattern: ".*"}}},
		{Type: types.DockerType, ProcessingRules: []types.ProcessingRule{{Type: types.ExcludeAtMatch}}},
		{Type: types.DockerType, ProcessingRules: []types.ProcessingRule{{Pattern: ".*"}}},
	}

	for _, config := range invalidConfigs {
		err := config.Validate()
		assert.NotNil(t, err)
	}
}

func TestCompileShouldSucceedWithValidRules(t *testing.T) {
	rules := []types.ProcessingRule{{Pattern: "[[:alnum:]]{5}", Type: types.IncludeAtMatch}}
	config := &LogsConfig{ProcessingRules: rules}
	err := config.Compile()
	assert.Nil(t, err)
	assert.NotNil(t, rules[0].Reg)
	assert.True(t, rules[0].Reg.MatchString("abcde"))
}

func TestCompileShouldFailWithInvalidRules(t *testing.T) {
	invalidRules := []types.ProcessingRule{
		{Type: types.IncludeAtMatch, Pattern: "(?=abf)"},
	}

	for _, rule := range invalidRules {
		config := &LogsConfig{ProcessingRules: []types.ProcessingRule{rule}}
		err := config.Compile()
		assert.NotNil(t, err)
		assert.Nil(t, rule.Reg)
	}
}
