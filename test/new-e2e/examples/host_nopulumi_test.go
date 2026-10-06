// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	hostinstallscript "github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/installscript"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const noPulumiCustomLogsConfig = `logs:
  - type: file
    path: /tmp/no-pulumi-installer.log
    service: no_pulumi_installer
    source: custom
`

type noPulumiHostInstallerSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestNoPulumiHostInstaller(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &noPulumiHostInstallerSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(
					ec2.WithOS(e2eos.Ubuntu2204E2E),
					ec2.WithInternetAccess(),
				),
				ec2.WithoutAgent(),
			),
		),
	))
}

func (s *noPulumiHostInstallerSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	require.Nil(s.T(), s.Env().Agent, "the provisioner must not install the Agent")
	_, err := s.Env().RemoteHost.Execute("test ! -e /etc/datadog-agent/datadog.yaml")
	require.NoError(s.T(), err, "Agent config exists before the no-Pulumi installer ran")

	s.T().Cleanup(func() {
		_, err := s.Env().RemoteHost.Execute("sudo apt-get remove -y --purge datadog-agent && sudo rm -rf /etc/datadog-agent")
		assert.NoError(s.T(), err, "failed to remove the Agent installed by the test")
	})

	require.NoError(s.T(), hostinstallscript.Install(context.Background(), s.Env(), hostinstallscript.Params{
		AgentVersion: "latest",
		AgentConfig:  "logs_enabled: true",
		Integrations: map[string]string{"custom_logs.d": noPulumiCustomLogsConfig},
	}))
	require.NotNil(s.T(), s.Env().Agent)
}

func (s *noPulumiHostInstallerSuite) TestAgentStatus() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		status, err := s.Env().Agent.Client.StatusWithError()
		require.NoError(c, err)
		assert.NotEmpty(c, status.Content, "Agent status returned no output")
	}, 2*time.Minute, 5*time.Second)
}

func (s *noPulumiHostInstallerSuite) TestLogsReachFakeIntake() {
	const message = "no-pulumi-host-installer-log"
	defer s.Env().RemoteHost.Execute("sudo rm -f /tmp/no-pulumi-installer.log")

	s.Env().RemoteHost.MustExecute("echo '" + message + "' | sudo tee /tmp/no-pulumi-installer.log >/dev/null")

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(
			"no_pulumi_installer",
			fakeintake.WithMessageContaining(message),
		)
		require.NoError(c, err)
		assert.NotEmpty(c, logs, "log produced after no-Pulumi installation did not reach fakeintake")
	}, 5*time.Minute, 10*time.Second)
}
