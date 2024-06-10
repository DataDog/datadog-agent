// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package secret contains e2e tests for secret management (runtime)
package secret

import (
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	secrets "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-shared-components/secretsutils"
)

type windowsRuntimeSecretSuite struct {
	baseRuntimeSecretSuite
}

func TestWindowsRuntimeSecretSuite(t *testing.T) {
	e2e.Run(t, &windowsRuntimeSecretSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
	)), e2e.WithDevMode())
}

func (v *windowsRuntimeSecretSuite) TestSecretRuntimeAPIKey() {
	config := `secret_backend_command: C:\TestFolder\wrapper.bat
hostname: ENC[hostname]`

	agentParams := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(config),
	}

	agentParams = append(agentParams, secrets.WithWindowsSecretSetupScript("C:/TestFolder/wrapper.bat", false)...)

	v.T().Log("creating secret client")
	secretClient := secrets.NewSecretClient(v.T(), v.Env().RemoteHost, "C:/TestFolder")
	v.T().Log("setting secret hostname")
	secretClient.SetSecret("hostname", "e2e.test")

	v.T().Log("updating env")
	v.UpdateEnv(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
			awshost.WithAgentOptions(agentParams...),
		),
	)

	v.T().Log("checking hostname")
	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		checks, err := v.Env().FakeIntake.Client().GetCheckRun("datadog.agent.up")
		require.NoError(t, err)
		if assert.NotEmpty(t, checks) {
			assert.Equal(t, "e2e.test", checks[len(checks)-1].HostName)
		}
	}, 30*time.Second, 2*time.Second)
}
