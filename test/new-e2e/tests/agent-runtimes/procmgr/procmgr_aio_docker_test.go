// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"path"
	"strconv"
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
	// Any non-empty value passes 01-check-apikey.sh.
	aioThrowawayAPIKey = "0123456789abcdef0123456789abcdef"

	aioContInitMarker90 = "cont-init-marker-90-bash-sh"
	aioContInitMarker91 = "cont-init-marker-91-no-suffix"
	aioContInitMarker92 = "cont-init-marker-92-symlink-target"
)

// aioContInitSetupScript creates the injected cont-init files in one remote
// shell. 92-linked is a symlink to the read-only mount path. The entrypoint
// copies every file before it runs any of them, so the link has to exist first.
const aioContInitSetupScript = `bash -s <<'EOS'
set -euo pipefail
dir=$(mktemp -d)
cat > "$dir/90-custom.sh" <<'EOF'
#!/bin/bash
echo ` + aioContInitMarker90 + `
EOF
chmod 0644 "$dir/90-custom.sh"
cat > "$dir/91-custom" <<'EOF'
#!/bin/sh
echo ` + aioContInitMarker91 + `
EOF
chmod 0644 "$dir/91-custom"
cat > "$dir/92-target" <<'EOF'
#!/bin/sh
echo ` + aioContInitMarker92 + `
EOF
chmod 0644 "$dir/92-target"
ln -s /mnt/ro/92-target "$dir/92-linked"
printf '%s\n' "$dir"
EOS
`

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

// BeforeTest starts the opt-in container again when an earlier test stopped it.
func (s *procmgrAIODockerSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	if s.containerStatus(s.T()) == "running" {
		return
	}
	s.Env().RemoteHost.MustExecute("docker start " + aioAgentContainer)
	s.waitForAgentRunning(s.T(), 3*time.Minute)
}

func (s *procmgrAIODockerSuite) TestSupervisorRunsTheAgent() {
	s.waitForAgentRunning(s.T(), 3*time.Minute)

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
	s.waitForAgentRunning(s.T(), 3*time.Minute)
	s.cleanupStartAgentContainer(s.T())

	s.Env().RemoteHost.MustExecute("docker stop -t " + aioStopTimeoutSec + " " + aioAgentContainer)

	exitCode := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		"docker inspect -f '{{.State.ExitCode}}' " + aioAgentContainer))
	s.Equal("0", exitCode, "SIGTERM should stop dd-procmgrd cleanly, 137 means Docker had to SIGKILL it")

	logs := s.Env().RemoteHost.MustExecute("docker logs --tail 200 " + aioAgentContainer + " 2>&1")
	s.Contains(logs, "dd-procmgrd stopped (exit code 0)")
}

// exit_with_supervisor: dd-procmgrd stops the other children before exiting 137,
// so the container can outlive the signal.
func (s *procmgrAIODockerSuite) TestCoreAgentDeathEndsTheContainer() {
	s.waitForAgentRunning(s.T(), 3*time.Minute)
	agentPID := fieldValue(s.describeProcess(s.T(), "datadog-agent"), "PID")
	requireHostPID(s.T(), agentPID)
	// The graceful stop of a running sibling is what proves the supervisor shut it down.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assertField(c, s.describeProcess(c, "datadog-agent-trace"), "State", "Running")
	}, time.Minute, 2*time.Second)
	s.cleanupStartAgentContainer(s.T())

	// Suite host clock, same machine as dockerd. time.Now() here would be the Go process.
	since := strings.TrimSpace(s.Env().RemoteHost.MustExecute(`date -u '+%Y-%m-%dT%H:%M:%S.%NZ'`))
	// describe's PID is a host PID because the provisioner sets pid: host.
	s.Env().RemoteHost.MustExecute("sudo kill -9 " + agentPID)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.Equal(c, "exited", s.containerStatus(c))
	}, 2*time.Minute, 2*time.Second, "container should exit after the core agent is killed")

	exitCode := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		"docker inspect -f '{{.State.ExitCode}}' " + aioAgentContainer))
	s.Equal("137", exitCode)

	logs := s.Env().RemoteHost.MustExecute(
		"docker logs --since " + shSingleQuote(since) + " " + aioAgentContainer + " 2>&1")
	s.Contains(logs, "[datadog-agent] critical process ended, shutting down with exit code 137")
	s.Contains(logs, "[datadog-agent-trace] sending graceful stop (stop requested)")
	s.Contains(logs, "dd-procmgrd stopped (exit code 137)")
	s.NotContains(logs, "force-killing")
	s.NotContains(logs, "still running after force-kill, giving up")
}

// datadog-agent-trace is already Running: apm_config.enabled defaults to true.
func (s *procmgrAIODockerSuite) TestTraceCrashRestarts() {
	s.waitForAgentRunning(s.T(), 3*time.Minute)

	var pid string
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out := s.describeProcess(c, "datadog-agent-trace")
		assertField(c, out, "State", "Running")
		pid = fieldValue(out, "PID")
		requireHostPID(c, pid)
	}, time.Minute, 2*time.Second)

	s.Env().RemoteHost.MustExecute("sudo kill -9 " + pid)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out := s.describeProcess(c, "datadog-agent-trace")
		assertField(c, out, "State", "Running")
		newPID := fieldValue(out, "PID")
		requireHostPID(c, newPID)
		assert.NotEqual(c, pid, newPID, "trace agent should come back with a different PID")
	}, time.Minute, 2*time.Second)

	s.Equal("running", s.containerStatus(s.T()))
	assertField(s.T(), s.describeProcess(s.T(), "datadog-agent"), "State", "Running")
}

// Stock image. Skipped is not Failed: the catalog skip reasons have to match.
func (s *procmgrAIODockerSuite) TestDisabledAuxStaysSkipped() {
	s.waitForAgentRunning(s.T(), 3*time.Minute)

	dataPlane := s.describeProcess(s.T(), "datadog-agent-data-plane")
	assertField(s.T(), dataPlane, "State", "Skipped")
	assertField(s.T(), dataPlane, "Skip Reasons", "config_gate")

	otel := s.describeProcess(s.T(), "datadog-agent-otel")
	assertField(s.T(), otel, "State", "Skipped")
	assertField(s.T(), otel, "Skip Reasons", "path_missing, config_gate")

	s.Equal("running", s.containerStatus(s.T()))
	assertField(s.T(), s.describeProcess(s.T(), "datadog-agent"), "State", "Running")
}

// The image HEALTHCHECK interval is 30s, so a restart stays starting for at
// least one interval.
func (s *procmgrAIODockerSuite) TestHealthcheckReportsHealthy() {
	s.waitForAgentRunning(s.T(), 3*time.Minute)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		_, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(aioAgentContainer, "/probe.sh")
		require.NoError(c, err, stderr)
		status := strings.TrimSpace(s.Env().RemoteHost.MustExecuteOn(c,
			"docker inspect -f '{{.State.Health.Status}}' "+aioAgentContainer))
		assert.Equal(c, "healthy", status)
	}, 2*time.Minute, 5*time.Second)
}

// The entrypoint exports DD_PM_CONFIG_DIR and children must not inherit it.
// DD_PM_SOCKET_PATH is kept.
func (s *procmgrAIODockerSuite) TestCoreAgentInheritsDDEnv() {
	s.waitForAgentRunning(s.T(), 3*time.Minute)
	pid := fieldValue(s.describeProcess(s.T(), "datadog-agent"), "PID")
	requireHostPID(s.T(), pid)

	environ, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
		aioAgentContainer, "cat", "/proc/"+pid+"/environ")
	s.Require().NoError(err, stderr)
	s.True(environHas(environ, "DD_DD_URL"))
	s.True(environHas(environ, "DD_PM_SOCKET_PATH"))
	s.False(environHas(environ, "DD_PM_CONFIG_DIR"))
}

// The :ro mount fails the entrypoint if cp stops following the symlink and
// chmod hits the target.
func (s *procmgrAIODockerSuite) TestCustomContInitRunsNonShAndSymlink() {
	dir := strings.TrimSpace(s.Env().RemoteHost.MustExecute(aioContInitSetupScript))
	s.Require().NotEmpty(dir)
	image := s.agentImage(s.T())
	name := "aio-cont-init-" + path.Base(dir)

	t := s.T()
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "rm -rf "+shSingleQuote(dir))
	})

	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker create --name %s -e ENTRYPOINT=procmgr-all-in-one -e DD_API_KEY=%s -v %s:/etc/cont-init.d/90-custom.sh:ro -v %s:/etc/cont-init.d/91-custom:ro -v %s:/mnt/ro/92-target:ro %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(dir+"/90-custom.sh"),
		shSingleQuote(dir+"/91-custom"),
		shSingleQuote(dir+"/92-target"),
		shSingleQuote(image),
	))
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "docker rm -fv "+shSingleQuote(name))
	})

	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"tar -C %s -cf - 92-linked | docker cp - %s:/etc/cont-init.d",
		shSingleQuote(dir), shSingleQuote(name)))
	// Check the copied entry while the container is only created. A started container
	// can exit early, which would make docker exec fail for an unrelated reason.
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker cp %s:/etc/cont-init.d/92-linked - | tar -tvf - | grep -q -- '92-linked -> /mnt/ro/92-target'",
		shSingleQuote(name)))
	s.Env().RemoteHost.MustExecute("docker start " + shSingleQuote(name))

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs := s.Env().RemoteHost.MustExecuteOn(c, "docker logs "+shSingleQuote(name)+" 2>&1")
		assertLogLinesInOrder(c, logs,
			"===== RUNNING /etc/cont-init.d/90-custom.sh =====",
			aioContInitMarker90,
			"===== RUNNING /etc/cont-init.d/91-custom =====",
			aioContInitMarker91,
			"===== RUNNING /etc/cont-init.d/92-linked =====",
			aioContInitMarker92,
			"===== STARTING dd-procmgrd =====",
		)
	}, 2*time.Minute, 2*time.Second)
}

// timeout 30: reaching dd-procmgrd would hang docker run.
func (s *procmgrAIODockerSuite) TestContInitFailsFastWithoutAPIKey() {
	image := s.agentImage(s.T())
	_, err := s.Env().RemoteHost.Execute(
		"timeout 30 docker run --rm -e ENTRYPOINT=procmgr-all-in-one " + shSingleQuote(image))
	s.Require().Error(err)
	s.NotContains(err.Error(), "status 124", "timeout means the entrypoint did not abort")
	s.Contains(err.Error(), "You must set either a DD_API_KEY or DD_DELEGATED_AUTH_ORG_UUID environment")
	s.NotContains(err.Error(), "===== STARTING dd-procmgrd =====")
}

func (s *procmgrAIODockerSuite) waitForAgentRunning(t require.TestingT, timeout time.Duration) {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			aioAgentContainer,
			"env", "DD_PM_SOCKET_PATH="+aioSocket, linuxCLIBin, "describe", "datadog-agent",
		)
		require.NoError(c, err, stderr)
		assertField(c, stdout, "State", "Running")
	}, timeout, 2*time.Second, "datadog-agent should be Running under dd-procmgrd")
}

// t is captured before Cleanup: inside Cleanup, s.T() is the parent.
func (s *procmgrAIODockerSuite) cleanupStartAgentContainer(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "docker start "+aioAgentContainer)
		s.waitForAgentRunning(t, 3*time.Minute)
	})
}

func (s *procmgrAIODockerSuite) describeProcess(t require.TestingT, name string) string {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
		aioAgentContainer,
		"env", "DD_PM_SOCKET_PATH="+aioSocket, linuxCLIBin, "describe", name,
	)
	require.NoError(t, err, stderr)
	return stdout
}

func (s *procmgrAIODockerSuite) containerStatus(t require.TestingT) string {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	return strings.TrimSpace(s.Env().RemoteHost.MustExecuteOn(t,
		"docker inspect -f '{{.State.Status}}' "+aioAgentContainer))
}

func (s *procmgrAIODockerSuite) agentImage(t require.TestingT) string {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	return strings.TrimSpace(s.Env().RemoteHost.MustExecuteOn(t,
		"docker inspect -f '{{.Config.Image}}' "+aioAgentContainer))
}

func requireHostPID(t require.TestingT, pid string) {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	_, err := strconv.ParseUint(pid, 10, 64)
	require.NoError(t, err, "describe PID %q", pid)
	require.NotEqual(t, "0", pid, "describe PID")
}

func environHas(environ, name string) bool {
	prefix := name + "="
	for _, entry := range strings.Split(environ, "\x00") {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

func assertLogLinesInOrder(t assert.TestingT, logs string, parts ...string) {
	rest := logs
	for _, part := range parts {
		idx := strings.Index(rest, part)
		if !assert.GreaterOrEqual(t, idx, 0, "logs missing %q:\n%s", part, logs) {
			return
		}
		rest = rest[idx+len(part):]
	}
}

func shSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
