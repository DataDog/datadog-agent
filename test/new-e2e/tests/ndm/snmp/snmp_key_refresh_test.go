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

	err := vm.CopyFolder("compose-key-refresh/data", "/tmp/data")
	v.Require().NoError(err)

	vm.CopyFile("compose-key-refresh/snmpCompose.yaml", "/tmp/snmpCompose.yaml")

	_, err = vm.Execute("docker-compose -f /tmp/snmpCompose.yaml up -d")
	v.Require().NoError(err)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetrics(c, fakeIntake)
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

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		lastAPIKey, err := fakeIntake.Client().GetLastAPIKey()
		assert.NoError(c, err)
		assert.Equal(c, lastAPIKey, apiKey1)
	}, 1*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetrics(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	secretClient.SetSecret("api_key", apiKey2)
	v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))

	time.Sleep(10 * time.Second)
	err = fakeIntake.Client().FlushServerAndResetAggregators()
	v.Require().NoError(err)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		lastAPIKey, err := fakeIntake.Client().GetLastAPIKey()
		assert.NoError(c, err)
		assert.Equal(c, lastAPIKey, apiKey2)
	}, 1*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetrics(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	fmt.Println("=====================")
	fmt.Println("=====================")
	fmt.Println("=====================")
	fmt.Println(fakeIntake.URL)
	fmt.Println("=====================")
	fmt.Println(fakeIntake.Port)
	fmt.Println("=====================")
	fmt.Println(fakeIntake.Host)
	fmt.Println("=====================")
	fmt.Println("=====================")
	fmt.Println("=====================")

	testMetric, err := fakeIntake.Client().FilterMetrics("snmp.sysUpTimeInstance")
	fmt.Println("=====================")
	fmt.Println("=====================")
	fmt.Println("=====================")
	fmt.Println(testMetric[0])
	fmt.Println("=====================")
	fmt.Println(testMetric[0].GetMetadata().String())
	fmt.Println("=====================")
	fmt.Println(testMetric[0].Tags)
	fmt.Println("=====================")
	fmt.Println("=====================")
	fmt.Println("=====================")
}

func checkBasicMetrics(c *assert.CollectT, fakeIntake *components.FakeIntake) {
	metrics, err := fakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metrics, "snmp.sysUpTimeInstance", "metrics %v doesn't contain snmp.sysUpTimeInstance", metrics)
}
