// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package secret contains e2e tests for secret management (runtime)
package secret

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

type windowsRuntimeSecretSuite struct {
	baseRuntimeSecretSuite
}

func TestWindowsRuntimeSecretSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsRuntimeSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault))),
	)))
}

func (v *windowsRuntimeSecretSuite) testSecretRuntimeHostname(wrapperDirectory string) {
	config := `secret_backend_command: ` + wrapperDirectory + `\wrapper.bat
secret_backend_arguments:
  - '` + wrapperDirectory + `'
hostname: ENC[hostname]`

	agentParams := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(config),
	}
	if strings.Contains(wrapperDirectory, "ProgramData") {
		agentParams = append(agentParams, secretsutils.WithWindowsSetupScriptNoPerms(wrapperDirectory+"/wrapper.bat")...)
	} else {
		agentParams = append(agentParams, secretsutils.WithWindowsSetupScript(wrapperDirectory+"/wrapper.bat", false)...)
	}

	secretClient := secretsutils.NewClient(v.T(), v.Env().RemoteHost, wrapperDirectory)
	secretClient.SetSecret("hostname", "e2e.test")

	v.UpdateEnv(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
				ec2.WithAgentOptions(agentParams...),
			),
		),
	)

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		checks, err := v.Env().FakeIntake.Client().GetCheckRun("datadog.agent.up")
		require.NoError(t, err)
		if assert.NotEmpty(t, checks) {
			assert.Equal(t, "e2e.test", checks[len(checks)-1].HostName)
		}
	}, 30*time.Second, 2*time.Second)
}

func (v *windowsRuntimeSecretSuite) TestSecretRuntimeHostname() {
	v.testSecretRuntimeHostname(`C:/TestFolder`)
}

func (v *windowsRuntimeSecretSuite) TestSecretRuntimeHostnameProgramData() {
	v.testSecretRuntimeHostname(`C:/ProgramData/DataDog/Test`)
}
