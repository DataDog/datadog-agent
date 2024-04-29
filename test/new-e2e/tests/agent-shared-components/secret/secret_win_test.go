// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package secret contains e2e tests for secret management (runtime)
package secret

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type windowsRuntimeSecretSuite struct {
	baseRuntimeSecretSuite
}

func TestWindowsRuntimeSecretSuite(t *testing.T) {
	e2e.Run(t, &windowsRuntimeSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
	)))
}

//go:embed fixtures/setup_secret.ps1
var secretSetupScript []byte

func (v *windowsRuntimeSecretSuite) TestSecretRuntimeAPIKey() {
	config := `secret_backend_command: C:\TestFolder\secret.bat
hostname: ENC[hostname]`

	v.UpdateEnv(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
			awshost.WithAgentOptions(
				agentparams.WithFile(`C:/TestFolder/setup_secret.ps1`, string(secretSetupScript), true),
			),
		),
	)

	v.Env().RemoteHost.MustExecute(`C:/TestFolder/setup_secret.ps1 -FilePath "C:/TestFolder/secret.bat" -FileContent '@echo {"hostname": {"value": "e2e.test"}}'`)

	v.UpdateEnv(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
			awshost.WithAgentOptions(agentparams.WithAgentConfig(config))),
	)

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		checks, err := v.Env().FakeIntake.Client().GetCheckRun("datadog.agent.up")
		require.NoError(t, err)
		if assert.NotEmpty(t, checks) {
			assert.Equal(t, "e2e.test", checks[len(checks)-1].HostName)
		}
	}, 30*time.Second, 2*time.Second)
}
