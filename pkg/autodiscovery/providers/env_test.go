// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/stretchr/testify/assert"
)

func TestEnvCollectEmpty(t *testing.T) {
	envProvider := NewEnvProvider()

	integrationConfigs, err := envProvider.Collect()
	assert.Nil(t, err)
	assert.Equal(t, 0, len(integrationConfigs))
}

func TestEnvCollectInvalid(t *testing.T) {
	// Missing closing parenthesis at type key
	config.Datadog.Set("logs_config.custom_configs", "[{\"type:\"tcp\",\"port\":1234,\"service\":\"fooService\",\"source\":\"barSource\",\"tags\":[\"foo:bar\",\"baz\"]}]")
	envProvider := NewEnvProvider()

	integrationConfigs, err := envProvider.Collect()
	assert.NotNil(t, err)
	assert.Equal(t, 0, len(integrationConfigs))
}

func TestEnvCollectValid(t *testing.T) {
	config.Datadog.Set("logs_config.custom_configs", "[{\"type\":\"tcp\",\"port\":1234,\"service\":\"fooService\",\"source\":\"barSource\",\"tags\":[\"foo:bar\",\"baz\"]}]")
	envProvider := NewEnvProvider()

	integrationConfigs, err := envProvider.Collect()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(integrationConfigs))

	var logConfig []logsConfig.LogsConfig
	err = json.Unmarshal(integrationConfigs[0].LogsConfig, &logConfig)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(logConfig))

	expectedConfig := logsConfig.LogsConfig{
		Type:    "tcp",
		Port:    1234,
		Service: "fooService",
		Source:  "barSource",
		Tags:    []string{"foo:bar", "baz"},
	}
	assert.Equal(t, expectedConfig, logConfig[0])
}

func TestEnvCollectMultipleValid(t *testing.T) {
	config.Datadog.Set("logs_config.custom_configs", "[{\"type\":\"tcp\",\"port\":1234,\"service\":\"fooService\",\"source\":\"barSource\",\"tags\":[\"foo:bar\",\"baz\"]}, {\"type\":\"file\",\"path\":\"/tmp/file.log\",\"service\":\"fooService\",\"source\":\"barSource\",\"tags\":[\"foo:bar\",\"log\"]}]")
	envProvider := NewEnvProvider()

	integrationConfigs, err := envProvider.Collect()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(integrationConfigs))

	var logConfig []logsConfig.LogsConfig
	err = json.Unmarshal(integrationConfigs[0].LogsConfig, &logConfig)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(logConfig))

	expectedConfig1 := logsConfig.LogsConfig{
		Type:    "tcp",
		Port:    1234,
		Service: "fooService",
		Source:  "barSource",
		Tags:    []string{"foo:bar", "baz"},
	}
	expectedConfig2 := logsConfig.LogsConfig{
		Type:    "file",
		Path:    "/tmp/file.log",
		Service: "fooService",
		Source:  "barSource",
		Tags:    []string{"foo:bar", "log"},
	}
	assert.Equal(t, expectedConfig1, logConfig[0])
	assert.Equal(t, expectedConfig2, logConfig[1])
}
