// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type autoDiscoverySuite struct {
	e2e.BaseSuite[environments.Host]
}

func autoDiscoverySuiteProvisioner(agentConfig string) provisioners.Provisioner {
	return awshost.Provisioner(
		awshost.WithRunOptions(
			scenec2.WithDocker(),
			scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)),
		),
	)
}

func TestAutoDiscoverySuite(t *testing.T) {
	e2e.Run(t, &autoDiscoverySuite{}, e2e.WithProvisioner(autoDiscoverySuiteProvisioner(``)))
}

func (v *autoDiscoverySuite) TestAutoDiscovery() {
	vm := v.Env().RemoteHost
	fakeIntake := v.Env().FakeIntake

	setupDevice(v.Require(), vm)

	// language=yaml
	agentConfig := `
network_devices:
  autodiscovery:
    loader: core
    configs:
      - network_address: 127.0.0.0/30
        port: 1161
        community_string: 'cisco-nexus'
`
	v.UpdateEnv(autoDiscoverySuiteProvisioner(agentConfig))

	err := fakeIntake.Client().FlushServerAndResetAggregators()
	v.Require().NoError(err)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetrics(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		ndmPayload := checkLastNDMPayload(c, fakeIntake, "default")
		require.NotEmpty(c, ndmPayload.Devices)
		checkCiscoNexusDeviceMetadata(c, ndmPayload.Devices[0])
	}, 2*time.Minute, 10*time.Second)
}

func (v *autoDiscoverySuite) TestAuthenticationsConfig() {
	vm := v.Env().RemoteHost
	fakeIntake := v.Env().FakeIntake

	setupDevice(v.Require(), vm)

	// language=yaml
	agentConfig := `
network_devices:
  autodiscovery:
    loader: core
    configs:
      - network_address: 127.0.0.0/30
        port: 1161
        authentications:
          - community_string: 'invalid1'
          - community_string: 'cisco-nexus'
          - community_string: 'invalid2'
`
	v.UpdateEnv(autoDiscoverySuiteProvisioner(agentConfig))

	err := fakeIntake.Client().FlushServerAndResetAggregators()
	v.Require().NoError(err)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetrics(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		ndmPayload := checkLastNDMPayload(c, fakeIntake, "default")
		require.NotEmpty(c, ndmPayload.Devices)
		checkCiscoNexusDeviceMetadata(c, ndmPayload.Devices[0])
	}, 2*time.Minute, 10*time.Second)

	cacheContent, err := vm.Execute("sudo cat /opt/datadog-agent/run/snmp/71beef32f1b72708")
	v.Require().NoError(err)
	v.Require().Equal(`[{"ip":"127.0.0.1","auth_index":1,"failures":0}]`, cacheContent)
}
