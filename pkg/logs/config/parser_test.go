// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseJSONStringWithValidFormatShouldSucceed(t *testing.T) {
	var config *LogsConfig
	var err error

	config, err = Parse(`[{}]`)
	assert.Nil(t, err)
	assert.NotNil(t, config)

	config, err = Parse(`[{"source":"any_source","service":"any_service"}]`)
	assert.Nil(t, err)
	assert.Equal(t, "any_source", config.Source)
	assert.Equal(t, "any_service", config.Service)

	config, err = Parse(`[{"source":"any_source","service":"any_service","log_processing_rules":[{"type":"multi_line","name":"numbers","pattern":"[0-9]"}]}]`)
	assert.Nil(t, err)
	assert.Equal(t, "any_source", config.Source)
	assert.Equal(t, "any_service", config.Service)
	assert.Equal(t, 1, len(config.ProcessingRules))

	rule := config.ProcessingRules[0]
	assert.Equal(t, "multi_line", rule.Type)
	assert.Equal(t, "numbers", rule.Name)
	assert.True(t, rule.Reg.MatchString("123"))
	assert.False(t, rule.Reg.MatchString("a123"))
}

func TestParseJSONStringWithInvalidFormatShouldFail(t *testing.T) {
	var config *LogsConfig
	var err error

	config, err = Parse(``)
	assert.NotNil(t, err)
	assert.Nil(t, config)

	config, err = Parse(`{}`)
	assert.NotNil(t, err)
	assert.Nil(t, config)

	config, err = Parse(`{\"source\":\"any_source\",\"service\":\"any_service\"}`)
	assert.NotNil(t, err)
	assert.Nil(t, config)
}
