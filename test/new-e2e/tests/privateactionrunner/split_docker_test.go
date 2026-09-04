// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
)

type dockerPARSplitSuite struct {
	e2e.BaseSuite[environments.DockerHost]

	signingKey testSigningKey
}

func TestDockerPARSplitSuite(t *testing.T) {
	t.Parallel()
	urn, privateKey := GenerateTestRunnerIdentity(t)
	suite := &dockerPARSplitSuite{
		signingKey: generateTestSigningKey(t, "docker-runner-key"),
	}

	e2e.Run(t, suite, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(scendocker.WithAgentOptions(
			dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_ENABLED", pulumi.String("true")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", pulumi.String("true")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL", pulumi.String("false")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_URN", pulumi.String(urn)),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY", pulumi.String(privateKey)),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST", pulumi.String(runCommandAction)),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_COMMANDS", pulumi.String(`["rshell:echo"]`)),
			dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_IDLE_TIMEOUT_SECONDS", pulumi.String("5")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", pulumi.String("true")),
		)),
	)))
}

func (s *dockerPARSplitSuite) TestAllInOneTopologyExecutesTask() {
	client := s.Env().FakeIntake.Client()
	s.Require().NoError(client.FlushPAR())
	s.waitForContainerProcessState("datadog-agent", parControlProcess, "Running", 2*time.Minute)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		count, err := client.GetPARDequeueCount()
		require.NoError(c, err)
		require.Greater(c, count, 0, "par-control should poll fakeintake")
	}, 2*time.Minute, 2*time.Second)

	setPARTaskSigningKey(s.T(), client, s.signingKey)
	taskID := uuid.New().String()
	s.Require().NoError(client.EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "echo par-split-docker-e2e",
		"allowedCommands": []string{"rshell:echo"},
	}))
	s.waitForContainerProcessState("datadog-agent", parExecutorProcess, "Running", 2*time.Minute)
	s.Require().NoError(client.RCAddConfig("", runnerKeysRCProduct, s.signingKey.id, s.signingKey.id, s.signingKey.config))

	result, err := client.GetPARTaskResult(taskID, 2*time.Minute)
	s.Require().NoError(err)
	s.Require().True(result.Success, "split PAR action failed: %+v", result)
	s.Require().Equal(0, rshellExitCode(s.T(), result), "unexpected rshell result: %+v", result)
	s.Require().Contains(result.Outputs["stdout"], "par-split-docker-e2e")
}

func (s *dockerPARSplitSuite) waitForContainerProcessState(container, process, state string, timeout time.Duration) {
	s.T().Helper()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			container,
			procmgrCLI, "describe", process,
		)
		require.NoError(c, err, stderr)
		require.Contains(c, strings.ReplaceAll(stdout, " ", ""), "State:"+state)
	}, timeout, 2*time.Second, "%s in %s should become %s", process, container, state)
}
