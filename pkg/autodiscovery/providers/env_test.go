// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestEnvCollectEmpty(t *testing.T) {
	envProvider := NewEnvVarProvider()

	integrationConfigs, err := envProvider.Collect()
	assert.Nil(t, err)
	assert.Equal(t, 0, len(integrationConfigs))
}

func TestEnvCollectValid(t *testing.T) {
	envVar := `[{"type":"tcp","port":1234,"service":"fooService","source":"barSource","tags":["foo:bar","baz"]}]`
	config.Datadog.Set("logs_config.custom_configs", envVar)
	envProvider := NewEnvVarProvider()

	integrationConfigs, err := envProvider.Collect()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(integrationConfigs))
	assert.Equal(t, []byte(envVar), []byte(integrationConfigs[0].LogsConfig))

	envVar = `[{"type":"tcp","port":1234,"service":"fooService","source":"barSource","tags":["foo:bar","baz"]}, {"type":"file","path":"/tmp/file.log","service":"fooService","source":"barSource","tags":["foo:bar","log"]}]`
	config.Datadog.Set("logs_config.custom_configs", envVar)
	integrationConfigs, err = envProvider.Collect()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(integrationConfigs))
	assert.Equal(t, []byte(envVar), []byte(integrationConfigs[0].LogsConfig))
}
