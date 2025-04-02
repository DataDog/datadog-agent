// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmp

import (
	_ "embed"
	"fmt"
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

//go:embed config-key-refresh/cisco-nexus.yaml
var snmpKeyRefreshConfig string

type snmpVMSuite struct {
	e2e.BaseSuite[environments.Host]
}

func snmpVMProvisioner(opts ...awshost.ProvisionerOption) provisioners.Provisioner {
	agentConfig := `
log_level: debug

network_devices:
  namespace: default
`

	allOpts := []awshost.ProvisionerOption{
		awshost.WithDocker(),
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithFile("/etc/datadog-agent/conf.d/snmp.d/snmp.yaml", snmpKeyRefreshConfig, true),
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

	vm.CopyFile("compose-key-refresh/snmpCompose.yaml", "/tmp/snmpCompose.yaml")

	_, err = vm.Execute("docker-compose -f /tmp/snmpCompose.yaml up -d")
	v.Require().NoError(err)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetric(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetadata(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

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
	}, 2*time.Minute, 10*time.Second)

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
	}, 2*time.Minute, 10*time.Second)
}

func checkBasicMetric(c *assert.CollectT, fakeIntake *components.FakeIntake) {
	metrics, err := fakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metrics, "snmp.sysUpTimeInstance", "metrics %v doesn't contain snmp.sysUpTimeInstance", metrics)
}

func checkBasicMetadata(c *assert.CollectT, fakeIntake *components.FakeIntake) {
	ndmPayloads, err := fakeIntake.Client().GetNDMPayloads()
	assert.NoError(c, err)
	assert.Greater(c, len(ndmPayloads), 0)

	for _, tmp := range ndmPayloads {
		fmt.Println("==========================")
		fmt.Println("==========================")
		fmt.Println("==========================")
		fmt.Println("==========================")
		fmt.Println("==========================")
		fmt.Println("==========================")
		fmt.Println("LEN: ", len(ndmPayloads))
		fmt.Println(tmp)
		fmt.Println(tmp.Devices)
	}

	ndmPayload := ndmPayloads[0]
	assert.Equal(c, ndmPayload.Namespace, "default")
	assert.Equal(c, string(ndmPayload.Integration), "snmp")
	assert.Greater(c, len(ndmPayload.Devices), 0)

	ciscoDevice := ndmPayload.Devices[0]
	assert.Equal(c, ciscoDevice.ID, "default:127.0.0.1")
	assert.Contains(c, ciscoDevice.IDTags, "snmp_device:127.0.0.1")
	assert.Contains(c, ciscoDevice.IDTags, "device_namespace:default")
	assert.Contains(c, ciscoDevice.Tags, "snmp_profile:cisco-nexus")
	assert.Contains(c, ciscoDevice.Tags, "device_vendor:cisco")
	assert.Contains(c, ciscoDevice.Tags, "snmp_device:127.0.0.1")
	assert.Contains(c, ciscoDevice.Tags, "device_namespace:default")
	assert.Equal(c, ciscoDevice.IPAddress, "127.0.0.1")
	assert.Equal(c, int32(ciscoDevice.Status), int32(1))
	assert.Equal(c, ciscoDevice.Name, "Nexus-eu1.companyname.managed")
	assert.Equal(c, ciscoDevice.Description, "oxen acted but acted kept")
	assert.Equal(c, ciscoDevice.SysObjectID, "1.3.6.1.4.1.9.12.3.1.3.1.2")
	assert.Equal(c, ciscoDevice.Location, "but kept Jaded their but kept quaintly driving their")
	assert.Equal(c, ciscoDevice.Profile, "cisco-nexus")
	assert.Equal(c, ciscoDevice.Vendor, "cisco")
	assert.Equal(c, ciscoDevice.DeviceType, "switch")
}
