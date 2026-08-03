// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	_ "embed"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	subconfig "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-subcommands/config"
)

type baseConfigSuite struct {
	e2e.BaseSuite[environments.Host]
	osOption scenec2.Option
}

func (v *baseConfigSuite) GetOs() scenec2.Option {
	return v.osOption
}

func getFullConfig(v *baseConfigSuite) map[interface{}]interface{} {
	output, err := v.Env().Agent.Client.ConfigWithError(
		agentclient.WithArgs([]string{"--all"}),
	)
	require.NoError(v.T(), err)

	var config map[interface{}]interface{}
	err = yaml.Unmarshal([]byte(output), &config)
	require.NoError(v.T(), err)

	return config
}

func (v *baseConfigSuite) TestDefaultConfig() {
	config := getFullConfig(v)

	subconfig.AssertConfigValueContains(v.T(), config, "api_key", "*******")
	subconfig.AssertConfigValueEqual(v.T(), config, "fips.enabled", false)
	subconfig.AssertConfigValueEqual(v.T(), config, "expvar_port", 5000)
	subconfig.AssertConfigValueEqual(v.T(), config, "network_devices.snmp_traps.forwarder.logs_no_ssl", false)
	subconfig.AssertConfigValueContains(v.T(), config, "cloud_provider_metadata", "aws")
}

//go:embed config/fixtures/datadog-agent.yaml
var configAgentConfiguration []byte

func (v *baseConfigSuite) TestNonDefaultConfig() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(
		awshost.WithRunOptions(
			v.GetOs(),
			scenec2.WithAgentOptions(agentparams.WithAgentConfig(string(configAgentConfiguration))),
		),
	))

	config := getFullConfig(v)

	subconfig.AssertConfigValueEqual(v.T(), config, "logs_enabled", false)
	subconfig.AssertConfigValueEqual(v.T(), config, "inventories_enabled", false)
	subconfig.AssertConfigValueEqual(v.T(), config, "inventories_min_interval", 1234)
	subconfig.AssertConfigValueEqual(v.T(), config, "inventories_max_interval", 3456)
	subconfig.AssertConfigValueEqual(v.T(), config, "expvar_port", 5678)
	subconfig.AssertConfigValueContains(v.T(), config, "tags", "e2e")
	subconfig.AssertConfigValueContains(v.T(), config, "tags", "test")
}
