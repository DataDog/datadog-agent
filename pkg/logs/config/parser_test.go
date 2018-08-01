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
	var configs []*LogsConfig
	var config *LogsConfig
	var err error

	configs, err = ParseJSON([]byte(`[{}]`))
	assert.Nil(t, err)
	config = configs[0]
	assert.NotNil(t, config)

	configs, err = ParseJSON([]byte(`[{"source":"any_source","service":"any_service"}]`))
	assert.Nil(t, err)
	config = configs[0]
	assert.Equal(t, "any_source", config.Source)
	assert.Equal(t, "any_service", config.Service)

	configs, err = ParseJSON([]byte(`[{"source":"any_source","service":"any_service","log_processing_rules":[{"type":"multi_line","name":"numbers","pattern":"[0-9]"}]}]`))
	assert.Nil(t, err)
	config = configs[0]
	assert.Equal(t, "any_source", config.Source)
	assert.Equal(t, "any_service", config.Service)
	assert.Equal(t, 1, len(config.ProcessingRules))

	rule := config.ProcessingRules[0]
	assert.Equal(t, "multi_line", rule.Type)
	assert.Equal(t, "numbers", rule.Name)
}

func TestParseJSONStringWithInvalidFormatShouldFail(t *testing.T) {
	var configs []*LogsConfig
	var err error

	configs, err = ParseJSON([]byte(``))
	assert.NotNil(t, err)
	assert.Nil(t, configs)

	configs, err = ParseJSON([]byte(`{}`))
	assert.NotNil(t, err)
	assert.Nil(t, configs)

	configs, err = ParseJSON([]byte(`{\"source\":\"any_source\",\"service\":\"any_service\"}`))
	assert.NotNil(t, err)
	assert.Nil(t, configs)
}
