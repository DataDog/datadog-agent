// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config contains helpers and e2e tests for config subcommand
package config

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type agentConfigSuite struct {
	e2e.BaseSuite[environments.Host]
}

var visibleConfigs = []string{
	"dogstatsd_capture_duration",
	"dogstatsd_stats",
	"log_level",
}

var hiddenConfigs = []string{
	"runtime_mutex_profile_fraction",
	"runtime_block_profile_rate",
	"log_payloads",
	"internal_profiling_goroutines",
	"internal_profiling",
}

func TestAgentConfigSuite(t *testing.T) {
	e2e.Run(t, &agentConfigSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func getFullConfig(v *agentConfigSuite) map[interface{}]interface{} {
	output, err := v.Env().Agent.Client.ConfigWithError()
	require.NoError(v.T(), err)

	var config map[interface{}]interface{}
	err = yaml.Unmarshal([]byte(output), &config)
	require.NoError(v.T(), err)

	return config
}

func (v *agentConfigSuite) TestDefaultConfig() {
	config := getFullConfig(v)

	assertConfigValueContains(v.T(), config, "api_key", "***************************")
	assertConfigValueEqual(v.T(), config, "fips.enabled", false)
	assertConfigValueEqual(v.T(), config, "expvar_port", "5000")
	assertConfigValueEqual(v.T(), config, "network_devices.snmp_traps.forwarder.logs_no_ssl", false)
	assertConfigValueContains(v.T(), config, "cloud_provider_metadata", "aws")
}

//go:embed fixtures/datadog-agent.yaml
var agentConfiguration []byte

func (v *agentConfigSuite) TestNonDefaultConfig() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig(string(agentConfiguration)))))

	config := getFullConfig(v)

	assertConfigValueEqual(v.T(), config, "logs_enabled", false)
	assertConfigValueEqual(v.T(), config, "inventories_enabled", false)
	assertConfigValueEqual(v.T(), config, "inventories_min_interval", 1234)
	assertConfigValueEqual(v.T(), config, "inventories_max_interval", 3456)
	assertConfigValueEqual(v.T(), config, "expvar_port", "5678")
	assertConfigValueContains(v.T(), config, "tags", "e2e")
	assertConfigValueContains(v.T(), config, "tags", "test")
}

func (v *agentConfigSuite) TestConfigListRuntime() {
	output := v.Env().Agent.Client.Config(agentclient.WithArgs([]string{"list-runtime"}))
	for _, config := range visibleConfigs {
		assert.Contains(v.T(), output, config)
	}

	for _, config := range hiddenConfigs {
		assert.NotContains(v.T(), output, config)
	}
}

func (v *agentConfigSuite) TestConfigGetDefault() {
	allRuntimeConfig := append(visibleConfigs, hiddenConfigs...)
	for _, config := range allRuntimeConfig {
		output := v.Env().Agent.Client.Config(agentclient.WithArgs([]string{"get", config}))
		assert.Contains(v.T(), output, fmt.Sprintf("%v is set to:", config))
	}
}

func (v *agentConfigSuite) TestConfigSetAndGet() {
	_, err := v.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"set", "log_level", "warn"}))
	assert.NoError(v.T(), err)
	output, _ := v.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"get", "log_level"}))
	assert.Contains(v.T(), output, "log_level is set to: warn")

	_, err = v.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"set", "log_level", "info"}))
	assert.NoError(v.T(), err)
	output = v.Env().Agent.Client.Config(agentclient.WithArgs([]string{"get", "log_level"}))
	assert.Contains(v.T(), output, "log_level is set to: info")
}

func (v *agentConfigSuite) TestConfigGetInvalid() {
	_, err := v.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"get", "dd_url"}))
	assert.Error(v.T(), err)

	_, err = v.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"get"}))
	assert.Error(v.T(), err)

	_, err = v.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"get", "too", "many", "args"}))
	assert.Error(v.T(), err)
}

func (v *agentConfigSuite) TestConfigSetInvalid() {
	_, err := v.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"set", "dd_url", "test"}))
	assert.Error(v.T(), err)

	_, err = v.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"set", "log_level"}))
	assert.Error(v.T(), err)

	_, err = v.Env().Agent.Client.ConfigWithError(agentclient.WithArgs([]string{"set", "dd_url", "too", "many", "args"}))
	assert.Error(v.T(), err)
}
