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
func aioDockerOptions(split bool, urn, privateKey string) []func(*dockeragentparams.Params) error {
	options := []func(*dockeragentparams.Params) error{
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
	if split {
		options = append(options, dockeragentparams.WithAgentServiceEnvVariable("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", pulumi.String("true")))
	}
	return options
}

type aioDockerMonolithSuite struct {
	e2e.BaseSuite[environments.DockerHost]

	signingKey testSigningKey
}

func TestProcmgrAIOMonolithPARSuite(t *testing.T) {
	t.Parallel()
	urn, privateKey := GenerateTestRunnerIdentity(t)
	suite := &aioDockerMonolithSuite{signingKey: generateTestSigningKey(t, "aio-monolith-key-"+uuid.NewString())}
	e2e.Run(t, suite, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(scendocker.WithAgentOptions(aioDockerOptions(false, urn, privateKey)...)),
	)))
}

func (s *aioDockerMonolithSuite) TestExecutesTask() {
	s.runAIOTask(aioMonolithProcess, false)
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
		awsdocker.WithRunOptions(scendocker.WithAgentOptions(aioDockerOptions(true, urn, privateKey)...)),
	)))
}

func (s *aioDockerSplitSuite) TestExecutesTask() {
	s.runAIOTask(parControlProcess, true)
}

type aioDockerTaskSuite interface {
	e2eEnv() *environments.DockerHost
	require() *require.Assertions
	eventually(fn func(*assert.CollectT), timeout, interval time.Duration)
	signing() testSigningKey
	testingT() *testing.T
}

func (s *aioDockerMonolithSuite) e2eEnv() *environments.DockerHost { return s.Env() }
func (s *aioDockerMonolithSuite) require() *require.Assertions     { return s.Require() }
func (s *aioDockerMonolithSuite) eventually(fn func(*assert.CollectT), timeout, interval time.Duration) {
	s.EventuallyWithT(fn, timeout, interval)
}
func (s *aioDockerMonolithSuite) signing() testSigningKey { return s.signingKey }
func (s *aioDockerMonolithSuite) testingT() *testing.T    { return s.T() }

func (s *aioDockerSplitSuite) e2eEnv() *environments.DockerHost { return s.Env() }
func (s *aioDockerSplitSuite) require() *require.Assertions     { return s.Require() }
func (s *aioDockerSplitSuite) eventually(fn func(*assert.CollectT), timeout, interval time.Duration) {
	s.EventuallyWithT(fn, timeout, interval)
}
func (s *aioDockerSplitSuite) signing() testSigningKey { return s.signingKey }
func (s *aioDockerSplitSuite) testingT() *testing.T    { return s.T() }

func (s *aioDockerMonolithSuite) runAIOTask(process string, split bool) {
	runAIODockerTask(s, process, split)
}

func (s *aioDockerSplitSuite) runAIOTask(process string, split bool) {
	runAIODockerTask(s, process, split)
}

func runAIODockerTask(s aioDockerTaskSuite, process string, split bool) {
	client := s.e2eEnv().FakeIntake.Client()
	s.require().NoError(client.FlushPAR())
	waitForAIOProcess(s, process, "Running", 3*time.Minute)
	if split {
		// describe keeps the unexpanded YAML. The spawned command line is the resolved path.
		assertLiveIPCCert(s, process)
		// Split stands the monolith down: it starts, then exits 0.
		waitForAIOProcess(s, aioMonolithProcess, "Exited", time.Minute)
	} else {
		waitForAIOProcess(s, parControlProcess, "Skipped", 30*time.Second)
	}

	key := s.signing()
	publishKey := func() {
		s.require().NoError(client.RCAddConfig("", runnerKeysRCProduct, key.id, key.id, key.config))
		setPARTaskSigningKey(s.testingT(), client, key)
	}
	// privateactionrunner run does not dequeue until the first AP_RUNNER_KEYS
	// callback. par-control polls with no key, so split publishes after that poll.
	if !split {
		publishKey()
	}

	s.eventually(func(c *assert.CollectT) {
		count, err := client.GetPARDequeueCount()
		require.NoError(c, err)
		require.Greater(c, count, 0, "PAR should poll fakeintake")
	}, 2*time.Minute, 2*time.Second)

	if split {
		publishKey()
	}
	// The runner can hold a key from an earlier attempt. Each tick signs a fresh task,
	// so one early rejection does not fail the wait.
	marker := "par-aio-" + process
	s.eventually(func(c *assert.CollectT) {
		require.NoError(c, client.RCAddConfig("", runnerKeysRCProduct, key.id, key.id, key.config))
		taskID := uuid.New().String()
		require.NoError(c, client.EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
			"command":         "echo " + marker,
			"allowedCommands": []string{"rshell:echo"},
		}))
		result, err := client.GetPARTaskResult(taskID, 20*time.Second)
		require.NoError(c, err)
		require.True(c, result.Success, "PAR action failed: %+v", result)
		require.Equal(c, 0, rshellExitCode(s.testingT(), result), "unexpected rshell result: %+v", result)
		require.Contains(c, result.Outputs["stdout"], marker)
	}, 2*time.Minute, 2*time.Second)

	if split {
		// The on-demand executor idles out after a minute, so check it right after the task.
		waitForAIOProcess(s, parExecutorProcess, "Running", 30*time.Second)
		assertLiveIPCCert(s, parExecutorProcess)
	}
}

func waitForAIOProcess(s aioDockerTaskSuite, process, state string, timeout time.Duration) {
	s.eventually(func(c *assert.CollectT) {
		out, stderr, err := s.e2eEnv().Docker.Client.ExecuteCommandStdoutStdErr(
			aioDockerContainer,
			"env", "DD_PM_SOCKET_PATH="+aioDockerSocket, aioDockerCLI, "describe", process,
		)
		require.NoError(c, err, stderr)
		require.Contains(c, strings.ReplaceAll(out, " ", ""), "State:"+state)
	}, timeout, 2*time.Second)
}

// The default config has no ipc_cert_file_path, so resolution lands beside datadog.yaml.
const aioIPCCertPath = "/etc/datadog-agent/ipc_cert.pem"

func assertLiveIPCCert(s aioDockerTaskSuite, process string) {
	s.eventually(func(c *assert.CollectT) {
		out, stderr, err := s.e2eEnv().Docker.Client.ExecuteCommandStdoutStdErr(
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
		cmdline, stderr, err := s.e2eEnv().Docker.Client.ExecuteCommandStdoutStdErr(
			aioDockerContainer,
			"sh", "-c", fmt.Sprintf("tr '\\000' ' ' < /proc/%d/cmdline", detail.PID),
		)
		require.NoError(c, err, stderr)
		require.Contains(c, cmdline, aioIPCCertPath)
		require.NotContains(c, cmdline, "${DD_PM_IPC_CERT_FILE_PATH}")
		require.NotContains(c, cmdline, "${DD_IPC_CERT_FILE_PATH}")
	}, 30*time.Second, 2*time.Second)
}
