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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
)

const (
	aioAgentContainer = "datadog-agent"
	// The all-in-one socket. The CLI falls back to it, but docker exec does not see the
	// entrypoint's socket export, so most tests pass it explicitly.
	aioSocket = "/opt/datadog-agent/run/dd-procmgrd.sock"
	// Above the 90s default stop_timeout, so a SIGKILL means a hung shutdown.
	aioStopTimeoutSec = "120"
	// Any non-empty value passes 01-check-apikey.sh.
	aioThrowawayAPIKey = "0123456789abcdef0123456789abcdef"
	// A bare docker run has no hostname source, so the core agent needs DD_HOSTNAME.
	aioBareHostname = "aio-bare-container"

	aioContInitMarker90 = "cont-init-marker-90-bash-sh"
	aioContInitMarker91 = "cont-init-marker-91-no-suffix"
	aioContInitMarker92 = "cont-init-marker-92-symlink-target"

	// A name the image does not ship, so the entrypoint treats the directory as custom.
	aioCustomService        = "custom-keepalive"
	aioCustomServiceCwd     = "custom-service-cwd="
	aioCustomServiceStaged  = "/services/" + aioCustomService
	aioCustomServiceZero    = "custom-service-zero="
	aioCustomServiceRun     = "/services/" + aioCustomService + "/run"
	aioCustomServiceSibling = "custom-service-sibling=keepalive-sibling"
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

// aioCustomServiceSetupScript writes a run script that prints its paths. The entrypoint runs
// a staged copy, so those paths are under services/<name>.
const aioCustomServiceSetupScript = `bash -s <<'EOS'
set -euo pipefail
dir=$(mktemp -d)
cat > "$dir/run" <<'EOF'
#!/bin/sh
echo "custom-service-cwd=$(pwd)"
echo "custom-service-zero=$0"
echo "custom-service-sibling=$(cat "$(dirname "$0")/sibling.txt")"
exec /bin/sleep 3600
EOF
echo keepalive-sibling > "$dir/sibling.txt"
chmod 0755 "$dir/run"
printf '%s\n' "$dir"
EOS
`

type procmgrAIODockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

// TestProcmgrAIODockerSuite runs the image without ENTRYPOINT, so _default starts dd-procmgrd.
func TestProcmgrAIODockerSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &procmgrAIODockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner()))
}

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

// exit_with_supervisor: dd-procmgrd stops the other children, then exits 137.
func (s *procmgrAIODockerSuite) TestCoreAgentDeathEndsTheContainer() {
	s.waitForAgentRunning(s.T(), 3*time.Minute)
	agentPID := fieldValue(s.describeProcess(s.T(), "datadog-agent"), "PID")
	requireHostPID(s.T(), agentPID)
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
	assertField(s.T(), otel, "Skip Reasons", "path_missing")

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

// The entrypoint exports DD_PM_CONFIG_DIR, which children must not inherit.
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
		"docker create --name %s -e DD_API_KEY=%s -v %s:/etc/cont-init.d/90-custom.sh:ro -v %s:/etc/cont-init.d/91-custom:ro -v %s:/mnt/ro/92-target:ro %s",
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

// The with-contenv shebang needs S6_KEEP_ENV=1, or the hook fails before dd-procmgrd starts.
func (s *procmgrAIODockerSuite) TestContInitWithContenvHookRunsWhenS6KeepEnvIsZero() {
	image := s.agentImage(s.T())
	name := "aio-cont-init-with-contenv"
	dir := strings.TrimSpace(s.Env().RemoteHost.MustExecute(`bash -s <<'EOS'
set -euo pipefail
dir=$(mktemp -d)
cat > "$dir/90-with-contenv" <<'EOF'
#!/usr/bin/with-contenv sh
printf '%s' "$AIO_CONTENV_VALUE" > /fork-marker/value
EOF
chmod 0755 "$dir/90-with-contenv"
printf '%s\n' "$dir"
EOS
`))
	s.Require().NotEmpty(dir)

	t := s.T()
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "rm -rf "+shSingleQuote(dir))
	})
	s.removeContainerNowAndOnCleanup(t, name)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker run -d --name %s -e DD_API_KEY=%s -e DD_HOSTNAME=%s -e S6_KEEP_ENV=0 -e AIO_CONTENV_VALUE=from-container -v %s:/etc/cont-init.d/90-with-contenv:ro -v %s:/fork-marker %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(aioBareHostname),
		shSingleQuote(dir+"/90-with-contenv"),
		shSingleQuote(dir),
		shSingleQuote(image),
	))

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		logs := s.Env().RemoteHost.MustExecuteOn(c, "docker logs "+shSingleQuote(name)+" 2>&1")
		assertLogLinesInOrder(c, logs,
			"===== RUNNING /etc/cont-init.d/90-with-contenv =====",
			"===== STARTING dd-procmgrd =====",
		)
	}, 2*time.Minute, 2*time.Second)

	value := s.Env().RemoteHost.MustExecute("cat " + shSingleQuote(dir+"/value"))
	s.Equal("from-container", strings.TrimSpace(value))
}

func (s *procmgrAIODockerSuite) TestCustomServiceStarts() {
	dir := strings.TrimSpace(s.Env().RemoteHost.MustExecute(aioCustomServiceSetupScript))
	s.Require().NotEmpty(dir)
	image := s.agentImage(s.T())
	name := "aio-custom-" + path.Base(dir)

	t := s.T()
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "rm -rf "+shSingleQuote(dir))
	})

	s.removeContainerNowAndOnCleanup(t, name)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker run -d --name %s -e DD_API_KEY=%s -e DD_HOSTNAME=%s -v %s:/etc/services.d/%s:ro %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(aioBareHostname),
		shSingleQuote(dir),
		aioCustomService,
		shSingleQuote(image),
	))

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		logs := s.Env().RemoteHost.MustExecuteOn(c, "docker logs "+shSingleQuote(name)+" 2>&1")
		assertLogLinesInOrder(c, logs,
			"===== ADDING custom service /etc/services.d/"+aioCustomService+" =====",
			"===== STARTING dd-procmgrd =====",
			aioCustomServiceCwd,
			aioCustomServiceStaged,
			aioCustomServiceZero,
			aioCustomServiceRun,
			aioCustomServiceSibling,
		)
		assert.Equal(c, 1, strings.Count(logs, "===== ADDING custom service "), "only the custom service should be added")
	}, 2*time.Minute, 2*time.Second)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		for _, process := range []string{"datadog-agent", aioCustomService} {
			stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
				name,
				"env", "DD_PM_SOCKET_PATH="+aioSocket, linuxCLIBin, "describe", process,
			)
			require.NoError(c, err, stderr)
			assertField(c, stdout, "State", "Running")
		}
	}, time.Minute, 2*time.Second)
}

// The generated service sets S6_KEEP_ENV=1, or its with-contenv run script fails and restarts.
func (s *procmgrAIODockerSuite) TestCustomServiceWithContenvRunsWhenS6KeepEnvIsZero() {
	const service = "custom-with-contenv"
	dir := strings.TrimSpace(s.Env().RemoteHost.MustExecute(`bash -s <<'EOS'
set -euo pipefail
dir=$(mktemp -d)
mkdir "$dir/service" "$dir/marker"
cat > "$dir/service/run" <<'EOF'
#!/usr/bin/with-contenv sh
printf '%s' "$AIO_CONTENV_VALUE" > /marker/value.tmp
mv /marker/value.tmp /marker/value
exec /bin/sleep 3600
EOF
chmod 0755 "$dir/service/run"
printf '%s\n' "$dir"
EOS
`))
	s.Require().NotEmpty(dir)
	image := s.agentImage(s.T())
	name := "aio-svc-contenv-" + path.Base(dir)

	t := s.T()
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "rm -rf "+shSingleQuote(dir))
	})

	s.removeContainerNowAndOnCleanup(t, name)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker run -d --name %s -e DD_API_KEY=%s -e DD_HOSTNAME=%s -e S6_KEEP_ENV=0 -e AIO_CONTENV_VALUE=from-container -v %s:/etc/services.d/%s:ro -v %s:/marker %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(aioBareHostname),
		shSingleQuote(dir+"/service"),
		service,
		shSingleQuote(dir+"/marker"),
		shSingleQuote(image),
	))

	describe := func(c require.TestingT) string {
		stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			name,
			"env", "DD_PM_SOCKET_PATH="+aioSocket, linuxCLIBin, "describe", service,
		)
		require.NoError(c, err, stderr)
		return stdout
	}

	var pid string
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		logs := s.Env().RemoteHost.MustExecuteOn(c, "docker logs "+shSingleQuote(name)+" 2>&1")
		assertLogLinesInOrder(c, logs,
			"===== ADDING custom service /etc/services.d/"+service+" =====",
			"===== STARTING dd-procmgrd =====",
		)
		value := s.Env().RemoteHost.MustExecuteOn(c, "cat "+shSingleQuote(dir+"/marker/value")+" 2>/dev/null || true")
		assert.Equal(c, "from-container", strings.TrimSpace(value))
		out := describe(c)
		assertField(c, out, "State", "Running")
		assertField(c, out, "Restarts", "0")
		pid = fieldValue(out, "PID")
		assert.NotEmpty(c, pid)
	}, 2*time.Minute, 2*time.Second)

	// A failing script would restart within this window.
	require.Never(t, func() bool {
		stdout, _, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			name,
			"env", "DD_PM_SOCKET_PATH="+aioSocket, linuxCLIBin, "describe", service,
		)
		return err != nil || fieldValue(stdout, "Restarts") != "0" || fieldValue(stdout, "PID") != pid
	}, 5*time.Second, 500*time.Millisecond, "the with-contenv service must not restart")
}

// A down file in a shipped service directory holds its catalog process at start. Only an
// explicit dd-procmgr start runs it.
func (s *procmgrAIODockerSuite) TestShippedDownMarkerKeepsProcessStopped() {
	dir := strings.TrimSpace(s.Env().RemoteHost.MustExecute(`bash -s <<'EOS'
set -euo pipefail
dir=$(mktemp -d)
touch "$dir/down"
printf '%s\n' "$dir"
EOS
`))
	s.Require().NotEmpty(dir)
	image := s.agentImage(s.T())
	name := "aio-shipped-down-" + path.Base(dir)
	const process = "datadog-agent-trace"

	t := s.T()
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "rm -rf "+shSingleQuote(dir))
	})
	s.removeContainerNowAndOnCleanup(t, name)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker run -d --name %s -e DD_API_KEY=%s -e DD_HOSTNAME=%s -v %s:/etc/services.d/trace:ro %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(aioBareHostname),
		shSingleQuote(dir),
		shSingleQuote(image),
	))

	run := func(c require.TestingT, args ...string) string {
		stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			name,
			append([]string{"env", "DD_PM_SOCKET_PATH=" + aioSocket, linuxCLIBin}, args...)...,
		)
		require.NoError(c, err, stderr)
		return stdout
	}

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assertField(c, run(c, "describe", "datadog-agent"), "State", "Running")
	}, 3*time.Minute, 2*time.Second)
	// Held processes are Skipped, not Stopped: the start pass declines them.
	held := run(t, "describe", process)
	assertField(t, held, "State", "Skipped")
	assertField(t, held, "Skip Reasons", "auto_start_false")

	// trace-agent would start within this window if the down marker were ignored.
	// Call docker exec directly: require calls FailNow, which must not run off the test goroutine.
	require.Never(t, func() bool {
		stdout, _, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			name,
			"env", "DD_PM_SOCKET_PATH="+aioSocket, linuxCLIBin, "describe", process,
		)
		return err != nil || fieldValue(stdout, "State") != "Skipped"
	}, 10*time.Second, time.Second, "the down marker should keep "+process+" held")

	run(t, "start", process)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assertField(c, run(c, "describe", process), "State", "Running")
	}, 2*time.Minute, 2*time.Second)
}

// UID 100 is dd-agent. Group 0 is root, which owns the group-writable runtime directories.
func (s *procmgrAIODockerSuite) TestNonRootUserRunsProcmgr() {
	image := s.agentImage(s.T())
	name := "aio-nonroot"

	t := s.T()
	s.removeContainerNowAndOnCleanup(t, name)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker run -d --name %s --user 100:0 -e DD_API_KEY=%s -e DD_HOSTNAME=%s %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(aioBareHostname),
		shSingleQuote(image),
	))

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		pid := strings.TrimSpace(s.Env().RemoteHost.MustExecuteOn(c,
			"docker inspect -f '{{.State.Pid}}' "+shSingleQuote(name)))
		requireHostPID(c, pid)
		comm := strings.TrimSpace(s.Env().RemoteHost.MustExecuteOn(c, "cat /proc/"+pid+"/comm"))
		assert.Equal(c, "dd-procmgrd", comm, "a non-root container should run dd-procmgrd, not fall back to s6")
		stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			name,
			"env", "DD_PM_SOCKET_PATH="+aioSocket, linuxCLIBin, "describe", "datadog-agent",
		)
		require.NoError(c, err, stderr)
		assertField(c, stdout, "State", "Running")
	}, 3*time.Minute, 2*time.Second)
}

// The script writes script.ready before it sleeps, so the stop waits for it. Exit 143 means the
// TERM handler ran. Exit 137 means Docker's grace period expired.
func (s *procmgrAIODockerSuite) TestDockerStopDuringContInit() {
	image := s.agentImage(s.T())
	name := "aio-stop-during-cont-init"
	dir := strings.TrimSpace(s.Env().RemoteHost.MustExecute(`bash -s <<'EOS'
set -euo pipefail
dir=$(mktemp -d)
cat > "$dir/90-block" <<'EOF'
#!/bin/sh
echo ready > /fork-marker/script.ready
exec /bin/sleep 300
EOF
chmod 0755 "$dir/90-block"
printf '%s\n' "$dir"
EOS
`))
	s.Require().NotEmpty(dir)

	t := s.T()
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "rm -rf "+shSingleQuote(dir))
	})
	s.removeContainerNowAndOnCleanup(t, name)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker run -d --name %s -e DD_API_KEY=%s -e DD_HOSTNAME=%s -v %s:/etc/cont-init.d/90-block:ro -v %s:/fork-marker %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(aioBareHostname),
		shSingleQuote(dir+"/90-block"),
		shSingleQuote(dir),
		shSingleQuote(image),
	))

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		logs := s.Env().RemoteHost.MustExecuteOn(c, "docker logs "+shSingleQuote(name)+" 2>&1")
		assert.Contains(c, logs, "===== RUNNING /etc/cont-init.d/90-block =====")
		ready := s.Env().RemoteHost.MustExecuteOn(c, "test -f "+shSingleQuote(dir+"/script.ready")+" && echo yes || echo no")
		assert.Equal(c, "yes", strings.TrimSpace(ready))
	}, 2*time.Minute, 2*time.Second)

	s.Env().RemoteHost.MustExecute("docker stop -t 20 " + shSingleQuote(name))
	exitCode := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		"docker inspect -f '{{.State.ExitCode}}' " + shSingleQuote(name)))
	s.Equal("143", exitCode, "docker stop during cont-init should exit 143, not 137")
}

// The entrypoint signals the script's process group, so a forked child gets TERM too.
func (s *procmgrAIODockerSuite) TestDockerStopDuringContInitKillsForkedChild() {
	image := s.agentImage(s.T())
	name := "aio-stop-during-cont-init-fork"
	dir := strings.TrimSpace(s.Env().RemoteHost.MustExecute(`bash -s <<'EOS'
set -euo pipefail
dir=$(mktemp -d)
cat > "$dir/90-block" <<'EOF'
#!/bin/sh
trap 'wait "$child"; exit 0' TERM
/bin/sh -c 'trap "echo term > /fork-marker/child.term; exit 0" TERM; echo ready > /fork-marker/child.ready; while :; do sleep 1; done' &
child=$!
wait "$child"
EOF
chmod 0755 "$dir/90-block"
printf '%s\n' "$dir"
EOS
`))
	s.Require().NotEmpty(dir)

	t := s.T()
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "rm -rf "+shSingleQuote(dir))
	})
	s.removeContainerNowAndOnCleanup(t, name)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker run -d --name %s -e DD_API_KEY=%s -e DD_HOSTNAME=%s -v %s:/etc/cont-init.d/90-block:ro -v %s:/fork-marker %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(aioBareHostname),
		shSingleQuote(dir+"/90-block"),
		shSingleQuote(dir),
		shSingleQuote(image),
	))

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		logs := s.Env().RemoteHost.MustExecuteOn(c, "docker logs "+shSingleQuote(name)+" 2>&1")
		assert.Contains(c, logs, "===== RUNNING /etc/cont-init.d/90-block =====")
		ready := s.Env().RemoteHost.MustExecuteOn(c, "test -f "+shSingleQuote(dir+"/child.ready")+" && echo yes || echo no")
		assert.Equal(c, "yes", strings.TrimSpace(ready))
	}, 2*time.Minute, 2*time.Second)

	s.Env().RemoteHost.MustExecute("docker stop -t 20 " + shSingleQuote(name))
	exitCode := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		"docker inspect -f '{{.State.ExitCode}}' " + shSingleQuote(name)))
	s.Equal("143", exitCode, "docker stop during a forked cont-init should exit 143")
	term := s.Env().RemoteHost.MustExecute("cat " + shSingleQuote(dir+"/child.term") + " 2>/dev/null || echo missing")
	s.Equal("term", strings.TrimSpace(term), "the forked child should receive SIGTERM from the cont-init process group")
}

// The entrypoint keeps the 5 second SIGKILL watchdog until the group is empty. Exit 137 means
// Docker's own timeout won.
func (s *procmgrAIODockerSuite) TestDockerStopDuringContInitKillsTermIgnoringChild() {
	image := s.agentImage(s.T())
	name := "aio-stop-during-cont-init-ignore"
	dir := strings.TrimSpace(s.Env().RemoteHost.MustExecute(`bash -s <<'EOS'
set -euo pipefail
dir=$(mktemp -d)
cat > "$dir/90-block" <<'EOF'
#!/bin/sh
trap 'exit 0' TERM
/bin/sh -c 'trap "" TERM; echo ready > /fork-marker/child.ready; while :; do sleep 1; done' &
while :; do sleep 1; done
EOF
chmod 0755 "$dir/90-block"
printf '%s\n' "$dir"
EOS
`))
	s.Require().NotEmpty(dir)

	t := s.T()
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, "rm -rf "+shSingleQuote(dir))
	})
	s.removeContainerNowAndOnCleanup(t, name)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker run -d --name %s -e DD_API_KEY=%s -e DD_HOSTNAME=%s -v %s:/etc/cont-init.d/90-block:ro -v %s:/fork-marker %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(aioBareHostname),
		shSingleQuote(dir+"/90-block"),
		shSingleQuote(dir),
		shSingleQuote(image),
	))

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		logs := s.Env().RemoteHost.MustExecuteOn(c, "docker logs "+shSingleQuote(name)+" 2>&1")
		assert.Contains(c, logs, "===== RUNNING /etc/cont-init.d/90-block =====")
		ready := s.Env().RemoteHost.MustExecuteOn(c, "test -f "+shSingleQuote(dir+"/child.ready")+" && echo yes || echo no")
		assert.Equal(c, "yes", strings.TrimSpace(ready))
	}, 2*time.Minute, 2*time.Second)

	started := time.Now()
	s.Env().RemoteHost.MustExecute("docker stop -t 20 " + shSingleQuote(name))
	elapsed := time.Since(started)
	exitCode := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		"docker inspect -f '{{.State.ExitCode}}' " + shSingleQuote(name)))
	s.Equal("143", exitCode, "docker stop during a TERM-ignoring cont-init child should exit 143")
	s.GreaterOrEqual(elapsed, 4*time.Second, "the watchdog should stay until it SIGKILLs the child")
	s.Less(elapsed, 15*time.Second, "the 5 second SIGKILL should finish before Docker's stop timeout")
}

// timeout 30: reaching dd-procmgrd would hang docker run.
func (s *procmgrAIODockerSuite) TestContInitFailsFastWithoutAPIKey() {
	image := s.agentImage(s.T())
	_, err := s.Env().RemoteHost.Execute(
		"timeout 30 docker run --rm " + shSingleQuote(image))
	s.Require().Error(err)
	s.NotContains(err.Error(), "status 124", "timeout means the entrypoint did not abort")
	s.Contains(err.Error(), "You must set either a DD_API_KEY or DD_DELEGATED_AUTH_ORG_UUID environment")
	s.NotContains(err.Error(), "===== STARTING dd-procmgrd =====")
}

func (s *procmgrAIODockerSuite) TestOptInEntrypointRunsProcmgrd() {
	image := s.agentImage(s.T())
	name := "aio-opt-in-entrypoint"

	t := s.T()
	s.removeContainerNowAndOnCleanup(t, name)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"docker run -d --name %s -e ENTRYPOINT=procmgr-all-in-one -e DD_API_KEY=%s -e DD_HOSTNAME=%s %s",
		shSingleQuote(name),
		shSingleQuote(aioThrowawayAPIKey),
		shSingleQuote(aioBareHostname),
		shSingleQuote(image),
	))

	// The main process is bash until cont-init finishes and the entrypoint execs dd-procmgrd.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		pid := strings.TrimSpace(s.Env().RemoteHost.MustExecuteOn(c,
			"docker inspect -f '{{.State.Pid}}' "+shSingleQuote(name)))
		requireHostPID(c, pid)
		comm := strings.TrimSpace(s.Env().RemoteHost.MustExecuteOn(c, "cat /proc/"+pid+"/comm"))
		assert.Equal(c, "dd-procmgrd", comm, "ENTRYPOINT=procmgr-all-in-one should exec dd-procmgrd")
	}, 2*time.Minute, 2*time.Second)
}

func (s *procmgrAIODockerSuite) waitForAgentRunning(t require.TestingT, timeout time.Duration) {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	// No DD_PM_SOCKET_PATH: the CLI falls back to the all-in-one socket path.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		stdout, stderr, err := s.Env().Docker.Client.ExecuteCommandStdoutStdErr(
			aioAgentContainer,
			linuxCLIBin, "describe", "datadog-agent",
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

// removeContainerNowAndOnCleanup removes a container left by an interrupted earlier run, so a
// fixed name does not collide, and removes it again when the test ends.
func (s *procmgrAIODockerSuite) removeContainerNowAndOnCleanup(t *testing.T, name string) {
	remove := "docker rm -fv " + shSingleQuote(name) + " >/dev/null 2>&1 || true"
	s.Env().RemoteHost.MustExecuteOn(t, remove)
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecuteOn(t, remove)
	})
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
