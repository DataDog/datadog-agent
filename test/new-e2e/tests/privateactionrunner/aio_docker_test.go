// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"encoding/json"
	"fmt"
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

const (
	aioDockerContainer = "datadog-agent"
	aioDockerCLI       = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	aioDockerSocket    = "/opt/datadog-agent/run/dd-procmgrd.sock"
	aioMonolithProcess = "datadog-agent-par"
)

// DD_PM_SOCKET_PATH stays unset so the entrypoint default applies.
func aioDockerOptions(urn, privateKey string) []func(*dockeragentparams.Params) error {
	return []func(*dockeragentparams.Params) error{
		dockeragentparams.WithAgentServiceEnvVariable("ENTRYPOINT", pulumi.String("procmgr-all-in-one")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_ENABLED", pulumi.String("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL", pulumi.String("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_HOSTNAME", pulumi.String("par-aio-docker-e2e")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_URN", pulumi.String(urn)),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY", pulumi.String(privateKey)),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST", pulumi.String(runCommandAction)),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_COMMANDS", pulumi.String(`["rshell:echo"]`)),
		dockeragentparams.WithAgentServiceEnvVariable("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", pulumi.String("true")),
	}
}

type aioDockerSplitSuite struct {
	e2e.BaseSuite[environments.DockerHost]

	signingKey testSigningKey
}

func TestProcmgrAIOSplitPARSuite(t *testing.T) {
	t.Parallel()
	urn, privateKey := GenerateTestRunnerIdentity(t)
	suite := &aioDockerSplitSuite{signingKey: generateTestSigningKey(t, "aio-split-key-"+uuid.NewString())}
	e2e.Run(t, suite, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(scendocker.WithAgentOptions(aioDockerOptions(urn, privateKey)...)),
	)))
}

func (s *aioDockerSplitSuite) TestExecutesTask() {
	client := s.Env().FakeIntake.Client()
	s.Require().NoError(client.FlushPAR())
	s.waitForAIOProcess(parControlProcess, "Running", 3*time.Minute)
	// describe keeps the unexpanded YAML; the live command line has the resolved path.
	s.assertLiveIPCCert(parControlProcess)
	s.waitForAIOProcess(aioMonolithProcess, "Exited", time.Minute)

	key := s.signingKey
	s.EventuallyWithT(func(c *assert.CollectT) {
		count, err := client.GetPARDequeueCount()
		require.NoError(c, err)
		require.Greater(c, count, 0, "PAR should poll fakeintake")
	}, 2*time.Minute, 2*time.Second)

	s.Require().NoError(client.RCAddConfig("", runnerKeysRCProduct, key.id, key.id, key.config))
	setPARTaskSigningKey(s.T(), client, key)
	// The runner can hold a key from an earlier attempt. Each tick signs a fresh task,
	// so one early rejection does not fail the wait.
	const marker = "par-aio-" + parControlProcess
	s.EventuallyWithT(func(c *assert.CollectT) {
		require.NoError(c, client.RCAddConfig("", runnerKeysRCProduct, key.id, key.id, key.config))
		taskID := uuid.New().String()
		require.NoError(c, client.EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
			"command":         "echo " + marker,
			"allowedCommands": []string{"rshell:echo"},
		}))
		result, err := client.GetPARTaskResult(taskID, 20*time.Second)
		require.NoError(c, err)
		require.True(c, result.Success, "PAR action failed: %+v", result)
		require.Equal(c, 0, rshellExitCode(s.T(), result), "unexpected rshell result: %+v", result)
		require.Contains(c, result.Outputs["stdout"], marker)
	}, 2*time.Minute, 2*time.Second)

	// The on-demand executor idles out after a minute, so check it right after the task.
	s.waitForAIOProcess(parExecutorProcess, "Running", 30*time.Second)
	s.assertLiveIPCCert(parExecutorProcess)
}

func (s *aioDockerSplitSuite) waitForAIOProcess(process, state string, timeout time.Duration) {
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			aioDockerContainer,
			"env", "DD_PM_SOCKET_PATH="+aioDockerSocket, aioDockerCLI, "describe", process,
		)
		require.NoError(c, err, stderr)
		require.Contains(c, strings.ReplaceAll(out, " ", ""), "State:"+state)
	}, timeout, 2*time.Second)
}

// The default config has no ipc_cert_file_path, so resolution lands beside datadog.yaml.
const aioIPCCertPath = "/etc/datadog-agent/ipc_cert.pem"

func (s *aioDockerSplitSuite) assertLiveIPCCert(process string) {
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			aioDockerContainer,
			"env", "DD_PM_SOCKET_PATH="+aioDockerSocket, aioDockerCLI, "describe", "--json", process,
		)
		require.NoError(c, err, stderr)
		var detail struct {
			State string `json:"state"`
			PID   uint32 `json:"pid"`
		}
		require.NoError(c, json.Unmarshal([]byte(out), &detail))
		require.Equal(c, "Running", detail.State)
		require.NotZero(c, detail.PID)
		cmdline, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			aioDockerContainer,
			"sh", "-c", fmt.Sprintf("tr '\\000' ' ' < /proc/%d/cmdline", detail.PID),
		)
		require.NoError(c, err, stderr)
		require.Contains(c, cmdline, aioIPCCertPath)
		require.NotContains(c, cmdline, "${DD_PM_IPC_CERT_FILE_PATH}")
		require.NotContains(c, cmdline, "${DD_IPC_CERT_FILE_PATH}")
	}, 30*time.Second, 2*time.Second)
}
