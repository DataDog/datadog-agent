// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"

	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

type dockerTestSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestDockerTestSuite(t *testing.T) {
	t.Parallel()

	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithExtraComposeManifest("fakeProcess", pulumi.String(fakeProcessCompose)),
	}

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsdocker.Provisioner(
			awsdocker.WithRunOptions(
				scendocker.WithAgentOptions(agentOpts...),
			),
		)),
	}

	e2e.Run(t, &dockerTestSuite{}, options...)
}

func (s *dockerTestSuite) TestDockerProcessCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		status := getAgentStatus(collect, s.Env().Agent.Client)

		// Process checks run in the core agent; verify the standalone process-agent is not running
		assert.NotEmpty(t, status.ProcessAgentStatus.Error, "status: %+v", status)
		assert.Empty(t, status.ProcessAgentStatus.Expvars.Map.EnabledChecks)

		// Verify the process component is running in the core agent
		assert.ElementsMatch(t, status.ProcessComponentStatus.Expvars.Map.EnabledChecks, []string{"process", "rtprocess", "service_discovery"})
	}, 2*time.Minute, 5*time.Second)

	// Flush fake intake to remove any early payloads
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		assertProcessCollected(c, payloads, false, "dd")
		assertContainersCollected(c, payloads, []string{"fake-process"})
	}, 2*time.Minute, 10*time.Second)

	// Verify the process-agent is not collected as it should not be running
	requireProcessNotCollected(t, payloads, "process-agent")
}

func (s *dockerTestSuite) TestProcessDiscoveryCheck() {
	t := s.T()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_DISCOVERY_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithExtraComposeManifest("fakeProcess", pulumi.String(fakeProcessCompose)),
	}

	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithRunOptions(scendocker.WithAgentOptions(agentOpts...))))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process_discovery"}, false)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessDiscoveryPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcessDiscoveries()
		assert.NoError(c, err, "failed to get process discovery payloads from fakeintake")
		assert.NotEmpty(c, payloads, "no process discovery payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessDiscoveryCollected(t, payloads, "dd")
}

func (s *dockerTestSuite) TestHostPasswdUsername() {
	t := s.T()
	// The existing fake-process image runs dd as root. Use that UID to exercise
	// a guaranteed collision without depending on the image's dd-agent UID.
	const hostUsername = "e2e-host-root"
	passwdPath := strings.TrimSpace(s.Env().RemoteHost.MustExecute("mktemp /tmp/e2e-process-passwd.XXXXXX"))
	t.Cleanup(func() {
		require.NoError(t, s.Env().RemoteHost.Remove(passwdPath))
	})
	_, err := s.Env().RemoteHost.WriteFile(passwdPath, []byte(hostUsername+":x:0:0::/root:/bin/sh\n"))
	require.NoError(t, err)

	// Exercise both payload paths on the existing Docker host. UpdateEnv
	// recreates the Agent with HOST_ETC present before its first lookup.
	for _, processCollection := range []bool{true, false} {
		agentOpts := []dockeragentparams.Option{
			dockeragentparams.WithAgentServiceEnvVariable("HOST_ETC", pulumi.String("/host/etc")),
			dockeragentparams.WithExtraVolumes(passwdPath + ":/host/etc/passwd:ro"),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.Bool(processCollection)),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", pulumi.Bool(!processCollection)),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", pulumi.Bool(false)),
			dockeragentparams.WithAgentServiceEnvVariable("DD_DISCOVERY_ENABLED", pulumi.Bool(false)),
			dockeragentparams.WithExtraComposeManifest("fakeProcess", pulumi.String(fakeProcessCompose)),
		}
		// Discovery runs at startup, then every four hours by default. Flush
		// before switching from process mode so its startup payload is retained.
		if !processCollection {
			require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
		}
		s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithRunOptions(scendocker.WithAgentOptions(agentOpts...))))

		// The host mount must not replace the image's local passwd database.
		rootEntry, err := s.Env().Docker.Client.ExecuteCommandWithErr(s.Env().Agent.ContainerName, "getent", "passwd", "0")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(rootEntry, "root:"), "image root account changed: %s", rootEntry)
		agentEntry, err := s.Env().Docker.Client.ExecuteCommandWithErr(s.Env().Agent.ContainerName, "getent", "passwd", "dd-agent")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(agentEntry, "dd-agent:"), "image dd-agent account missing: %s", agentEntry)

		// Process payloads arrive frequently; flush after readiness to discard
		// anything buffered by the previous Agent without the host passwd mount.
		if processCollection {
			require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
		}
		s.EventuallyWithT(func(c *assert.CollectT) {
			var users []*model.ProcessUser
			if processCollection {
				payloads, err := s.Env().FakeIntake.Client().GetProcesses()
				require.NoError(c, err)
				for _, proc := range FilterProcessPayloadsByName(payloads, "dd") {
					users = append(users, proc.User)
				}
			} else {
				payloads, err := s.Env().FakeIntake.Client().GetProcessDiscoveries()
				require.NoError(c, err)
				for _, payload := range payloads {
					for _, proc := range payload.ProcessDiscoveries {
						if len(proc.Command.Args) > 0 && proc.Command.Args[0] == "dd" {
							users = append(users, proc.User)
						}
					}
				}
			}
			require.NotEmpty(c, users, "no dd process received (process collection enabled: %t)", processCollection)
			for _, user := range users {
				require.NotNil(c, user)
				assert.Equal(c, int32(0), user.Uid, "workload UID must be preserved")
				assert.Equal(c, hostUsername, user.Name, "host passwd must win over the image's root account")
			}
		}, 2*time.Minute, 10*time.Second)
	}
}

func (s *dockerTestSuite) TestProcessCheckWithIO() {
	t := s.T()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_PROCESS_ENABLED", pulumi.StringPtr("true")),
	}
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithRunOptions(scendocker.WithAgentOptions(agentOpts...))))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process", "rtprocess", "service_discovery"}, true)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

		assertProcessCollected(c, payloads, true, "dd")
	}, 2*time.Minute, 10*time.Second)
}

func (s *dockerTestSuite) TestProcessChecksWithNPM() {
	t := s.T()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_NETWORK_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_NETWORK_CONFIG_DIRECT_SEND", pulumi.StringPtr("false")),

		dockeragentparams.WithExtraComposeManifest("fakeProcess", pulumi.String(fakeProcessCompose)),
	}
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithRunOptions(scendocker.WithAgentOptions(agentOpts...))))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process", "rtprocess", "service_discovery", "connections"}, false)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

		assertProcessCollected(c, payloads, false, "dd")
		assertContainersCollected(c, payloads, []string{"fake-process"})
	}, 2*time.Minute, 10*time.Second)
}

func (s *dockerTestSuite) TestManualProcessCheck() {
	check := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName,
		"agent", "processchecks", "process", "--json")

	assertManualProcessCheck(s.T(), check, false, "dd", "fake-process")
}

func (s *dockerTestSuite) TestManualProcessDiscoveryCheck() {
	check := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName,
		"agent", "processchecks", "process_discovery", "--json")

	assertManualProcessDiscoveryCheck(s.T(), check, "dd")
}

func (s *dockerTestSuite) TestManualProcessCheckWithIO() {
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_PROCESS_ENABLED", pulumi.StringPtr("true")),
	}
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithRunOptions(scendocker.WithAgentOptions(agentOpts...))))

	check := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName,
		"agent", "processchecks", "process", "--json")

	assertManualProcessCheck(s.T(), check, true, "dd")
}
