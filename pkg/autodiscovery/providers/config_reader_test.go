// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

func TestGetIntegrationConfig(t *testing.T) {
	// file does not exist
	_, err := GetIntegrationConfigFromFile("foo", "")
	assert.NotNil(t, err)

	// file contains invalid Yaml
	_, err = GetIntegrationConfigFromFile("foo", "tests/invalid.yaml")
	assert.NotNil(t, err)

	// valid yaml, invalid configuration file
	config, err := GetIntegrationConfigFromFile("foo", "tests/notaconfig.yaml")
	assert.NotNil(t, err)
	assert.Equal(t, len(config.Instances), 0)

	// valid metric file
	config, err = GetIntegrationConfigFromFile("foo", "tests/metrics.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, config.MetricConfig)

	// valid logs-agent file
	config, err = GetIntegrationConfigFromFile("foo", "tests/logs-agent_only.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, config.LogsConfig)

	// valid configuration file
	config, err = GetIntegrationConfigFromFile("foo", "tests/testcheck.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.Name, "foo")
	assert.Equal(t, []byte(config.InitConfig), []byte("- test: 21\n"))
	assert.Equal(t, config.Source, "file:tests/testcheck.yaml")
	assert.Equal(t, len(config.Instances), 1)
	assert.Equal(t, []byte(config.Instances[0]), []byte("foo: bar\n"))
	assert.Len(t, config.ADIdentifiers, 0)
	assert.Nil(t, config.MetricConfig)
	assert.Nil(t, config.LogsConfig)

	// autodiscovery
	config, err = GetIntegrationConfigFromFile("foo", "tests/ad.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.ADIdentifiers, []string{"foo_id", "bar_id"})

	// advanced autodiscovery
	config, err = GetIntegrationConfigFromFile("foo", "tests/advanced_ad.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.AdvancedADIdentifiers, []integration.AdvancedADIdentifier{{KubeService: integration.KubeNamespacedName{Name: "svc-name", Namespace: "svc-ns"}}})

	// autodiscovery: check if we correctly refuse to load if a 'docker_images' section is present
	config, err = GetIntegrationConfigFromFile("foo", "tests/ad_deprecated.yaml")
	assert.NotNil(t, err)
}

func TestReadConfigFiles(t *testing.T) {
	paths := []string{"tests"}
	ResetReader(paths)

	configs, errors, err := ReadConfigFiles(GetAll)
	require.Nil(t, err)
	require.Equal(t, 17, len(configs))
	require.Equal(t, 3, len(errors))

	configs, _, err = ReadConfigFiles(WithoutAdvancedAD)
	require.Nil(t, err)
	require.Equal(t, 16, len(configs))

	configs, _, err = ReadConfigFiles(WithAdvancedADOnly)
	require.Nil(t, err)
	require.Equal(t, 1, len(configs))
	require.Equal(t, configs[0].AdvancedADIdentifiers, []integration.AdvancedADIdentifier{{KubeService: integration.KubeNamespacedName{Name: "svc-name", Namespace: "svc-ns"}}})

	configs, _, err = ReadConfigFiles(func(c integration.Config) bool { return c.Name == "baz" })
	require.Nil(t, err)
	require.Equal(t, 1, len(configs))
	require.Equal(t, configs[0].Name, "baz")
}
