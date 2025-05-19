// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmp

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

//go:embed config-vm/cisco-nexus.yaml
var snmpVMConfig string

type snmpVMSuite struct {
	e2e.BaseSuite[environments.Host]
}

func snmpVMProvisioner(opts ...awshost.ProvisionerOption) provisioners.Provisioner {
	allOpts := []awshost.ProvisionerOption{
		awshost.WithDocker(),
		awshost.WithAgentOptions(
			agentparams.WithFile("/etc/datadog-agent/conf.d/snmp.d/snmp.yaml", snmpVMConfig, true),
		),
	}
	allOpts = append(allOpts, opts...)

	return awshost.Provisioner(
		allOpts...,
	)
}

func TestSnmpVMSuite(t *testing.T) {
	e2e.Run(t, &snmpVMSuite{}, e2e.WithProvisioner(snmpVMProvisioner()))
}

func (v *snmpVMSuite) TestAPIKeyRefresh() {
	vm := v.Env().RemoteHost
	fakeIntake := v.Env().FakeIntake

	err := vm.CopyFolder("compose/data", "/tmp/data")
	v.Require().NoError(err)

	vm.CopyFile("compose-vm/snmpCompose.yaml", "/tmp/snmpCompose.yaml")

	_, err = vm.Execute("docker-compose -f /tmp/snmpCompose.yaml up -d")
	v.Require().NoError(err)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetric(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetadata(c, fakeIntake)
	}, 6*time.Minute, 10*time.Second)

	apiKey1 := strings.Repeat("0", 32)
	apiKey2 := strings.Repeat("1", 32)

	secretClient := secretsutils.NewClient(v.T(), v.Env().RemoteHost, "/tmp/test-secret")
	secretClient.SetSecret("api_key", apiKey1)

	agentConfig := `
api_key: ENC[api_key]

secret_backend_command: /tmp/test-secret/secret-resolver.py
secret_backend_arguments:
  - /tmp/test-secret
`

	v.UpdateEnv(
		snmpVMProvisioner(
			awshost.WithAgentOptions(
				agentparams.WithAgentConfig(agentConfig),
				secretsutils.WithUnixSetupScript("/tmp/test-secret/secret-resolver.py", false),
				agentparams.WithSkipAPIKeyInConfig(),
			),
		),
	)

	err = fakeIntake.Client().FlushServerAndResetAggregators()
	v.Require().NoError(err)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		lastAPIKey, err := fakeIntake.Client().GetLastAPIKey()
		assert.NoError(c, err)
		assert.Equal(c, lastAPIKey, apiKey1, "Last API key should be the initial API key")
	}, 1*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetric(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetadata(c, fakeIntake)
	}, 6*time.Minute, 10*time.Second)

	secretClient.SetSecret("api_key", apiKey2)
	v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))

	err = fakeIntake.Client().FlushServerAndResetAggregators()
	v.Require().NoError(err)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		lastAPIKey, err := fakeIntake.Client().GetLastAPIKey()
		assert.NoError(c, err)
		assert.Equal(c, lastAPIKey, apiKey2, "Last API key should be the new API key after refresh")
	}, 1*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetric(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetadata(c, fakeIntake)
	}, 6*time.Minute, 10*time.Second)
}

func checkBasicMetric(c *assert.CollectT, fakeIntake *components.FakeIntake) {
	metrics, err := fakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metrics, "snmp.sysUpTimeInstance", "Metrics %v doesn't contain snmp.sysUpTimeInstance", metrics)
}

func checkBasicMetadata(c *assert.CollectT, fakeIntake *components.FakeIntake) {
	ndmPayloads, err := fakeIntake.Client().GetNDMPayloads()
	assert.NoError(c, err)
	assert.Greater(c, len(ndmPayloads), 0)

	ndmPayload := ndmPayloads[len(ndmPayloads)-1]
	assert.Equal(c, "default", ndmPayload.Namespace)
	assert.Equal(c, "snmp", ndmPayload.Integration)
	assert.Greater(c, len(ndmPayload.Devices), 0)
	assert.Greater(c, len(ndmPayload.Interfaces), 0)

	ciscoDevice := ndmPayload.Devices[0]
	assert.Equal(c, "default:127.0.0.1", ciscoDevice.ID)
	assert.Contains(c, ciscoDevice.IDTags, "snmp_device:127.0.0.1")
	assert.Contains(c, ciscoDevice.IDTags, "device_namespace:default")
	assert.Contains(c, ciscoDevice.Tags, "snmp_profile:cisco-nexus")
	assert.Contains(c, ciscoDevice.Tags, "device_vendor:cisco")
	assert.Contains(c, ciscoDevice.Tags, "snmp_device:127.0.0.1")
	assert.Contains(c, ciscoDevice.Tags, "device_namespace:default")
	assert.Equal(c, "127.0.0.1", ciscoDevice.IPAddress)
	assert.Equal(c, int32(1), ciscoDevice.Status)
	assert.Equal(c, "Nexus-eu1.companyname.managed", ciscoDevice.Name)
	assert.Equal(c, "oxen acted but acted kept", ciscoDevice.Description)
	assert.Equal(c, "1.3.6.1.4.1.9.12.3.1.3.1.2", ciscoDevice.SysObjectID)
	assert.Equal(c, "but kept Jaded their but kept quaintly driving their", ciscoDevice.Location)
	assert.Equal(c, "cisco-nexus", ciscoDevice.Profile)
	assert.Equal(c, "cisco", ciscoDevice.Vendor)
	assert.Equal(c, "switch", ciscoDevice.DeviceType)
}
