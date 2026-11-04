// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgr

import (
	"strings"
	"testing"
	"time"

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
	aioAgentContainer = "datadog-agent"
	// docker exec does not see the entrypoint's socket export, so the CLI gets it explicitly.
	aioSocket = "/opt/datadog-agent/run/dd-procmgrd.sock"
	// Above the 90s default stop_timeout, so a SIGKILL means a hung shutdown.
	aioStopTimeoutSec = "120"
)

type procmgrAIODockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

// TestProcmgrAIODockerSuite runs the image with ENTRYPOINT=procmgr-all-in-one, so dd-procmgrd
// supervises the Agent instead of s6.
func TestProcmgrAIODockerSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &procmgrAIODockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(scendocker.WithAgentOptions(
			dockeragentparams.WithAgentServiceEnvVariable("ENTRYPOINT", pulumi.String("procmgr-all-in-one")),
		)),
	)))
}

func (s *procmgrAIODockerSuite) TestSupervisorRunsTheAgent() {
	s.waitForAgentRunning(3 * time.Minute)

	pid := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		"docker inspect -f '{{.State.Pid}}' " + aioAgentContainer))
	comm := strings.TrimSpace(s.Env().RemoteHost.MustExecute("cat /proc/" + pid + "/comm"))
	s.Equal("dd-procmgrd", comm, "the container's main process should be dd-procmgrd")

	logs := s.Env().RemoteHost.MustExecute("docker logs " + aioAgentContainer + " 2>&1")
	s.Contains(logs, "===== STARTING dd-procmgrd =====")

	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.agent.running")
		require.NoError(c, err)
		assert.NotEmpty(c, metrics, "the supervised Agent has not reported yet")
	}, 3*time.Minute, 10*time.Second)
}

func (s *procmgrAIODockerSuite) TestDockerStopShutsDownGracefully() {
	s.waitForAgentRunning(3 * time.Minute)
	s.T().Cleanup(func() {
		s.Env().RemoteHost.MustExecute("docker start " + aioAgentContainer)
		s.waitForAgentRunning(3 * time.Minute)
	})

	s.Env().RemoteHost.MustExecute("docker stop -t " + aioStopTimeoutSec + " " + aioAgentContainer)

	exitCode := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		"docker inspect -f '{{.State.ExitCode}}' " + aioAgentContainer))
	s.Equal("0", exitCode, "SIGTERM should stop dd-procmgrd cleanly, 137 means Docker had to SIGKILL it")

	logs := s.Env().RemoteHost.MustExecute("docker logs --tail 200 " + aioAgentContainer + " 2>&1")
	s.Contains(logs, "dd-procmgrd stopped (exit code 0)")
}

func (s *procmgrAIODockerSuite) waitForAgentRunning(timeout time.Duration) {
	s.T().Helper()
	s.EventuallyWithT(func(c *assert.CollectT) {
		stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			aioAgentContainer,
			"env", "DD_PM_SOCKET_PATH="+aioSocket, linuxCLIBin, "describe", "datadog-agent",
		)
		require.NoError(c, err, stderr)
		assertField(c, stdout, "State", "Running")
	}, timeout, 2*time.Second, "datadog-agent should be Running under dd-procmgrd")
}
