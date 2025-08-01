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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

type authsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestAuthsSuite(t *testing.T) {
	// language=yaml
	agentConfig := `
network_devices:
  autodiscovery:
    loader: core
    configs:
      - network_address: 127.0.0.0/30
        port: 1161
        community_string: 'public'
        authentications:
          - community_string: 'invalid1'
          - community_string: 'public'
          - community_string: 'invalid2'
`

	e2e.Run(t, &authsSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithDocker(),
			awshost.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)),
		),
	))
}

func (v *authsSuite) TestDeviceReachable() {
	vm := v.Env().RemoteHost
	fakeIntake := v.Env().FakeIntake

	err := vm.CopyFolder("compose/data", "/tmp/data")
	v.Require().NoError(err)

	vm.CopyFile("compose-vm/snmpCompose.yaml", "/tmp/snmpCompose.yaml")

	_, err = vm.Execute("docker-compose -f /tmp/snmpCompose.yaml up -d")
	v.Require().NoError(err)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetrics(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		ndmPayload := checkLastNDMPayload(c, fakeIntake, "default")
		checkGenericDeviceMetadata(c, ndmPayload.Devices[0])
	}, 2*time.Minute, 10*time.Second)
}
