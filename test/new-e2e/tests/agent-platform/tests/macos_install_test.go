// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
)

const (
	// macosAgentBinary and macosAgentStatusCmd are the CLI entrypoints used throughout this suite.
	macosAgentBinary    = "sudo /usr/local/bin/datadog-agent"
	macosAgentStatusCmd = macosAgentBinary + " status"

	// Ports and paths of the installed agent.
	macosAgentAPIPort        = 5001
	macosGUIPort             = 5002
	macosAuthTokenPath       = "/opt/datadog-agent/etc/auth_token"
	macosConfDefaultConfPath = "/opt/datadog-agent/etc"
	macosSysprobeSocketPath  = "/opt/datadog-agent/run/sysprobe.sock"
)

// Markers delimit each test's appended config block for removal during cleanup.
const (
	macosStatusAndConfigMarker  = "# added by e2e TestAgentStatusAndConfig"
	macosCPUMetricsMarker       = "# added by e2e TestCpuReportsSignalMetrics"
	macosDogstatsdMarker        = "# added by e2e TestDogstatsdListening"
	macosDogstatsdE2EMarker     = "# added by e2e TestDogstatsdMetricEndToEnd"
	macosProcessAgentDataMarker = "# added by e2e TestProcessAgentReportsProcessData"
	macosAPMTraceMarker         = "# added by e2e TestAPMTraceEndToEnd"
	macosNPMConfigMarker        = "# added by e2e TestNPMTracesConnection"
	// macosNPMProcessConfigMarker is duplicated rather than shared with macosProcessAgentDataMarker
	// so TestNPMTracesConnection doesn't depend on TestProcessAgentReportsProcessData having run.
	macosNPMProcessConfigMarker = "# added by e2e TestNPMTracesConnection process config"
)

// macosStatusAndConfigSanityTag is round-tripped through config, runtime config, and status
// output to prove the config-reload pipeline works end-to-end.
const macosStatusAndConfigSanityTag = "e2e-sanity:macos"

// macosProcessAgentSentinelProcess is a distinctive long-lived process searched for by name in
// fakeintake's process payloads.
const macosProcessAgentSentinelProcess = "ddprocsentinel"

// macosAPMSentinelService is a distinctive service name attached to the test trace, searched
// for in fakeintake's trace payloads.
const macosAPMSentinelService = "ddapmsentinel"

// macosBaseIntegrationPackage (datadog_checks_base) is always installed alongside the agent,
// so it's safe to query without network/CDN access.
const macosBaseIntegrationPackage = "datadog-checks-base"

// macosEssentialChecks are the core checks every default macOS install schedules, backing
// system.cpu/mem/disk/net/load/uptime/ntp metrics. Hardware- or runtime-context-dependent
// checks (battery, wlan, containerd, kubelet, ...) are excluded since they don't run on a
// bare EC2 host.
var macosEssentialChecks = []string{
	"cloud_hostinfo", "container_image", "container_lifecycle",
	"cpu", "disk", "io", "load", "memory", "network", "ntp", "telemetry", "uptime",
}

type macosInstallSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestMacosInstallScript(t *testing.T) {
	extraConfigMap := runner.ConfigMap{}
	// Pulumi needs to pick a smaller subnet subset on macOS; only settable via the configmap.
	extraConfigMap.Set("ddinfra:aws/useMacosCompatibleSubnets", "true", false)
	e2e.Run(t, &macosInstallSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.MacOSDefault)), ec2.WithoutAgent()),
			awshost.WithExtraConfigParams(extraConfigMap),
		)),
	)
}

// SetupSuite installs the agent once for all Test methods, so removePreInstalledAgent wipes
// any stale prior install first.
func (m *macosInstallSuite) SetupSuite() {
	m.BaseSuite.SetupSuite()

	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	removePreInstalledAgent(m.T(), macosTestClient)

	install.MacOS(m.T(), macosTestClient, installparams.WithUsername(m.Env().RemoteHost.Username), installparams.WithArch("x64"))
	if m.T().Failed() {
		m.FailNow("agent install failed, aborting SetupSuite")
	}

	// The agent should start at some point
	m.macosWaitForHealthyAgent(macosTestClient)
}

// removePreInstalledAgent mirrors cmd/agent/macos/uninstall_mac_os.sh's cleanup; best-effort,
// logs rather than fails on error.
func removePreInstalledAgent(t *testing.T, client *common.MacOSTestClient) {
	cmd := `
sudo launchctl bootout system/com.datadoghq.agent 2>/dev/null || true
sudo launchctl bootout system/com.datadoghq.sysprobe 2>/dev/null || true
sudo launchctl bootout system/com.datadoghq.data-plane 2>/dev/null || true
for logged_uid in $(who | awk '{print $1}' | sort -u | xargs -I{} id -u {} 2>/dev/null); do
	sudo launchctl bootout "gui/$logged_uid/com.datadoghq.gui" 2>/dev/null || true
	sudo launchctl bootout "gui/$logged_uid/com.datadoghq.ai-usage-agent.desktop-monitor" 2>/dev/null || true
	sudo launchctl bootout "gui/$logged_uid/com.datadoghq.ai-prompt-logger.desktop-monitor" 2>/dev/null || true
done
sudo pkill -f 'Datadog Agent.app' 2>/dev/null || true
sudo pkill -f 'ai-usage-agent-native-host.*--desktop-monitor' 2>/dev/null || true
sudo pkill -f 'ai-prompt-logger-native-host.*--desktop-monitor' 2>/dev/null || true
sudo rm -f /Library/LaunchDaemons/com.datadoghq.agent.plist
sudo rm -f /Library/LaunchDaemons/com.datadoghq.sysprobe.plist
sudo rm -f /Library/LaunchDaemons/com.datadoghq.data-plane.plist
sudo rm -f /Library/LaunchAgents/com.datadoghq.gui.plist
sudo rm -f /Library/LaunchAgents/com.datadoghq.ai-usage-agent.desktop-monitor.plist
sudo rm -f /Library/LaunchAgents/com.datadoghq.ai-prompt-logger.desktop-monitor.plist
sudo rm -rf "/Applications/Datadog Agent.app"
sudo rm -rf /opt/datadog-agent
sudo rm -f /usr/local/bin/datadog-agent
sudo rm -f /var/log/datadog
sudo rm -rf /private/var/root/datadog-install
`
	if _, err := client.Execute(cmd); err != nil {
		t.Logf("removePreInstalledAgent: cleanup command reported an error (may be harmless if no agent was installed): %v", err)
	}
}

// macosWaitForHealthyAgent polls until the agent is healthy. Safe in t.Cleanup: CollectT
// recovers failures internally, unlike MustExecuteOn(m.T(), ...).
func (m *macosInstallSuite) macosWaitForHealthyAgent(client *common.MacOSTestClient) {
	m.EventuallyWithT(func(c *assert.CollectT) {
		client.MustExecuteOn(c, macosAgentStatusCmd)
	}, 20*time.Second, 1*time.Second)
}

// macosRestartDaemon kickstarts restartTarget; logs rather than aborts on failure so later
// cleanup steps still run.
func (m *macosInstallSuite) macosRestartDaemon(client *common.MacOSTestClient, restartTarget string) {
	if _, err := client.Execute("sudo launchctl kickstart -k " + restartTarget); err != nil {
		m.T().Logf("cleanup: failed to restart %s: %v", restartTarget, err)
	}
}

// macosAppendConfigBlock idempotently appends a marker+block to confFilePath via a quoted
// heredoc (block must have no trailing newline). Caller owns reverting via
// macosRevertConfigBlock.
func (m *macosInstallSuite) macosAppendConfigBlock(client *common.MacOSTestClient, confFilePath, marker, block string) {
	client.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q %s || cat <<'EOF' | sudo tee -a %s
%s
%s
EOF`, marker, confFilePath, confFilePath, marker, block,
	))
}

// macosRevertConfigBlock removes the marker line plus block's line count (derived from block,
// not hand-counted). Logs rather than aborts on failure, safe in t.Cleanup.
func (m *macosInstallSuite) macosRevertConfigBlock(client *common.MacOSTestClient, confFilePath, marker, block string) {
	deleteLineCount := strings.Count(block, "\n") + 1
	if _, err := client.Execute(fmt.Sprintf(`sudo sed -i '' "/%s/,+%dd" %s`, marker, deleteLineCount, confFilePath)); err != nil {
		m.T().Logf("cleanup: failed to remove config block for marker %q in %s: %v", marker, confFilePath, err)
	}
}

// macosPatchConfigAndRestart appends a config block, kickstarts restartTarget, and waits for
// health, reverting on cleanup. Both cleanups are registered before the mutating calls
// (MustExecuteOn aborts on failure, which would skip cleanup registered after it). LIFO order:
// restart+wait registered first, revert second, so the revert runs before the restart that's
// meant to pick it up.
func (m *macosInstallSuite) macosPatchConfigAndRestart(client *common.MacOSTestClient, confFilePath, marker, block, restartTarget string) {
	m.T().Cleanup(func() {
		m.macosRestartDaemon(client, restartTarget)
		m.macosWaitForHealthyAgent(client)
	})
	m.T().Cleanup(func() {
		m.macosRevertConfigBlock(client, confFilePath, marker, block)
	})

	m.macosAppendConfigBlock(client, confFilePath, marker, block)
	client.MustExecuteOn(m.T(), "sudo launchctl kickstart -k "+restartTarget)
	m.macosWaitForHealthyAgent(client)
}

// macosGUIAuthenticatedClient performs the GUI login handshake (an intent token fetched from
// the core agent API, then the GUI's cookie-based /auth exchange) and returns an authenticated client.
func macosGUIAuthenticatedClient(t require.TestingT, host *components.RemoteHost, authToken string) *http.Client {
	intentClient := host.NewHTTPClient()
	intentReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/agent/gui/intent", macosAgentAPIPort), nil)
	require.NoError(t, err)
	intentReq.Header.Set("Authorization", "Bearer "+authToken)

	intentResp, err := intentClient.Do(intentReq)
	require.NoError(t, err)
	defer intentResp.Body.Close()
	require.Equal(t, http.StatusOK, intentResp.StatusCode)

	intentToken, err := io.ReadAll(intentResp.Body)
	require.NoError(t, err)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	guiClient := host.NewHTTPClient()
	guiClient.Jar = jar

	authURL := fmt.Sprintf("http://localhost:%d/auth?%s", macosGUIPort, url.Values{"intent": {string(intentToken)}}.Encode())
	authResp, err := guiClient.Get(authURL)
	require.NoError(t, err)
	defer authResp.Body.Close()
	require.Equal(t, http.StatusOK, authResp.StatusCode)

	return guiClient
}

// macosLaunchdPID returns the current pid of a launchd job, as reported by `launchctl print`.
func macosLaunchdPID(t require.TestingT, client *common.MacOSTestClient, label string) int {
	out, err := client.Execute("sudo launchctl print " + label)
	require.NoError(t, err)

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid = ") {
			pid, err := strconv.Atoi(strings.TrimPrefix(line, "pid = "))
			require.NoError(t, err)
			return pid
		}
	}
	require.Fail(t, "pid not found in launchctl print output for "+label)
	return 0
}

// enableSysprobeForRestartTest enables traceroute and kickstarts sysprobe. Every module ships
// disabled by default, so sysprobe exits and stays down (launchd won't relaunch after a clean
// exit); the GUI's /agent-restart handler lives in sysprobe's HTTP API, so it must be running
// for testAgentRestart.
func (m *macosInstallSuite) enableSysprobeForRestartTest(macosTestClient *common.MacOSTestClient) {
	const marker = "# added by e2e testAgentRestart"
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q /opt/datadog-agent/etc/system-probe.yaml || printf '\n%s\ntraceroute:\n  enabled: true\n' | sudo tee -a /opt/datadog-agent/etc/system-probe.yaml`,
		marker, marker,
	))
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.sysprobe")

	m.EventuallyWithT(func(c *assert.CollectT) {
		macosLaunchdPID(c, macosTestClient, "system/com.datadoghq.sysprobe")
		// Wait for the sysprobe socket to be available, since testAgentRestart dials it directly.
		macosTestClient.MustExecuteOn(c, "sudo test -S "+macosSysprobeSocketPath)
	}, 20*time.Second, 1*time.Second)
}

// testAgentRestart exercises the GUI's restart action end-to-end: GUI -> system-probe
// (over its unix socket) -> launchctl kickstart of the agent and sysprobe LaunchDaemons.
func (m *macosInstallSuite) testAgentRestart(macosTestClient *common.MacOSTestClient) {
	authTokenOutput, err := macosTestClient.Execute("sudo cat " + macosAuthTokenPath)
	require.NoError(m.T(), err)
	authToken := strings.TrimSpace(authTokenOutput)

	guiClient := macosGUIAuthenticatedClient(m.T(), m.Env().RemoteHost, authToken)

	agentPIDBefore := macosLaunchdPID(m.T(), macosTestClient, "system/com.datadoghq.agent")
	sysprobePIDBefore := macosLaunchdPID(m.T(), macosTestClient, "system/com.datadoghq.sysprobe")

	restartReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%d/agent/restart", macosGUIPort), nil)
	require.NoError(m.T(), err)
	restartResp, err := guiClient.Do(restartReq)
	require.NoError(m.T(), err)
	defer restartResp.Body.Close()
	body, err := io.ReadAll(restartResp.Body)
	require.NoError(m.T(), err)
	assert.Equal(m.T(), http.StatusOK, restartResp.StatusCode)
	assert.Equal(m.T(), "Success", strings.TrimSpace(string(body)))

	m.EventuallyWithT(func(c *assert.CollectT) {
		agentPIDAfter := macosLaunchdPID(c, macosTestClient, "system/com.datadoghq.agent")
		sysprobePIDAfter := macosLaunchdPID(c, macosTestClient, "system/com.datadoghq.sysprobe")
		assert.NotEqual(c, agentPIDBefore, agentPIDAfter, "agent launchd job should have restarted with a new pid")
		assert.NotEqual(c, sysprobePIDBefore, sysprobePIDAfter, "sysprobe launchd job should have restarted with a new pid")
	}, 30*time.Second, 2*time.Second)

	// the agent should be healthy again after the restart
	m.macosWaitForHealthyAgent(macosTestClient)
}

func (m *macosInstallSuite) TestInstallAgent() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)

	_, err := macosTestClient.Execute("sudo test -x /opt/datadog-agent/embedded/bin/agent-data-plane")
	assert.NoError(m.T(), err)
	_, err = macosTestClient.Execute("sudo test -f /Library/LaunchDaemons/com.datadoghq.data-plane.plist")
	assert.NoError(m.T(), err)
	_, err = macosTestClient.Execute("sudo launchctl print system/com.datadoghq.data-plane")
	assert.NoError(m.T(), err)

	// no world-writable files in /opt/datadog-agent, except run/ipc (intentionally writable for multi-user GUI sockets)
	worldWritableFiles, err := macosTestClient.Execute("sudo find /opt/datadog-agent \\( -type f -o -type d \\) -perm -002 ! -path '/opt/datadog-agent/run/ipc' ! -path '/opt/datadog-agent/run/ipc/*'")
	assert.NoError(m.T(), err)
	assert.Empty(m.T(), strings.TrimSpace(worldWritableFiles))
}

func (m *macosInstallSuite) TestAgentStatusAndConfig() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"

	// set a distinctive, verifiable config value and reload
	m.macosPatchConfigAndRestart(macosTestClient, confFilePath, macosStatusAndConfigMarker,
		"tags:\n  - "+macosStatusAndConfigSanityTag, "system/com.datadoghq.agent")

	// Status: functional signals, not just "the command didn't error".
	statusOutput, err := macosTestClient.Execute(macosAgentBinary + " status")
	assert.NoError(m.T(), err)
	statusOutput = common.SanitizeStatusOutputForKnownNoise(statusOutput)
	assert.NotContains(m.T(), statusOutput, "ERROR")
	assert.Contains(m.T(), statusOutput, "Forwarder")
	assert.Contains(m.T(), statusOutput, "Host Info")
	assert.Contains(m.T(), statusOutput, "DogStatsD")
	assert.Contains(m.T(), statusOutput, macosStatusAndConfigSanityTag)

	// poll since the first check cycle may not have completed right after restart (mirrors CheckAgentBehaviour on Linux/Windows)
	m.EventuallyWithT(func(c *assert.CollectT) {
		jsonStatus, err := macosTestClient.Execute(macosAgentBinary + " status -j")
		if !assert.NoError(c, err) {
			return
		}
		var statusMap map[string]any
		if !assert.NoError(c, json.Unmarshal([]byte(jsonStatus), &statusMap)) {
			return
		}
		runnerStats, ok := statusMap["runnerStats"].(map[string]any)
		if !assert.True(c, ok, "status JSON should contain runnerStats") {
			return
		}
		checks, ok := runnerStats["Checks"].(map[string]any)
		if !assert.True(c, ok, "runnerStats should contain Checks") {
			return
		}
		assert.NotEmpty(c, checks, "at least one check should be running")
	}, 20*time.Second, 1*time.Second)

	// exercises runtime settings API; tags isn't a gettable setting, so use log_level instead
	m.T().Cleanup(func() {
		if _, err := macosTestClient.Execute(macosAgentBinary + " config set log_level info"); err != nil {
			m.T().Logf("cleanup: failed to reset log_level: %v", err)
		}
	})
	_, err = macosTestClient.Execute(macosAgentBinary + " config set log_level debug")
	assert.NoError(m.T(), err)
	logLevelOutput, err := macosTestClient.Execute(macosAgentBinary + " config get log_level")
	assert.NoError(m.T(), err)
	assert.Contains(m.T(), logLevelOutput, "debug")

	// agent version: content check, not just exit code.
	versionOutput, err := macosTestClient.Execute(macosAgentBinary + " version")
	assert.NoError(m.T(), err)
	assert.Regexp(m.T(), `Agent \d+\.\d+\.\d+`, versionOutput)
}

// TestEssentialChecksLoaded asserts the core host-metric checks are actually scheduled, not
// just that some check runs. Read-only against SetupSuite's install.
func (m *macosInstallSuite) TestEssentialChecksLoaded() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)

	// scheduling is staggered on startup, so poll rather than assert once
	m.EventuallyWithT(func(c *assert.CollectT) {
		jsonStatus, err := macosTestClient.Execute(macosAgentBinary + " status -j")
		if !assert.NoError(c, err) {
			return
		}
		var statusMap map[string]any
		if !assert.NoError(c, json.Unmarshal([]byte(jsonStatus), &statusMap)) {
			return
		}
		runnerStats, ok := statusMap["runnerStats"].(map[string]any)
		if !assert.True(c, ok, "status JSON should contain runnerStats") {
			return
		}
		checks, ok := runnerStats["Checks"].(map[string]any)
		if !assert.True(c, ok, "runnerStats should contain Checks") {
			return
		}
		for _, name := range macosEssentialChecks {
			assert.Contains(c, checks, name, "essential check %q should be scheduled", name)
		}
	}, 40*time.Second, 2*time.Second)
}

// TestCpuReportsSignalMetrics proves the cpu check not only runs (TestEssentialChecksLoaded)
// but forwards real data: redirects dd_url to fakeintake and asserts a cpu metric arrives.
func (m *macosInstallSuite) TestCpuReportsSignalMetrics() {
	require.NoError(m.T(), m.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	m.macosPatchConfigAndRestart(macosTestClient, confFilePath, macosCPUMetricsMarker,
		"dd_url: "+fakeIntakeURL, "system/com.datadoghq.agent")

	// async delivery, so poll rather than assert once
	m.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := m.Env().FakeIntake.Client().FilterMetrics("system.cpu.idle")
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEmpty(c, metrics, "system.cpu.idle should be forwarded to fakeintake")
	}, 2*time.Minute, 5*time.Second)
}

// TestDogstatsdListening proves DogStatsD's UDP listener not only binds 8125 but receives,
// aggregates, and forwards a real metric.
func (m *macosInstallSuite) TestDogstatsdListening() {
	require.NoError(m.T(), m.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	boundPorts := macosTestClient.MustExecuteOn(m.T(), "sudo lsof -nP -iUDP:8125")
	assert.Contains(m.T(), boundPorts, "agent", "the agent process should be bound to UDP 8125")

	m.macosPatchConfigAndRestart(macosTestClient, confFilePath, macosDogstatsdMarker,
		"dd_url: "+fakeIntakeURL, "system/com.datadoghq.agent")

	const metricName = "e2e.macos.dogstatsd.sanity"
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`bash -c 'echo -n "%s:1|c" > /dev/udp/127.0.0.1/8125'`, metricName,
	))

	// async delivery, so poll rather than assert once
	m.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := m.Env().FakeIntake.Client().FilterMetrics(metricName)
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEmpty(c, metrics, "%s should be forwarded to fakeintake", metricName)
	}, 2*time.Minute, 5*time.Second)
}

// TestDogstatsdMetricEndToEnd extends TestDogstatsdListening to gauge/count/histogram metric
// types and tag propagation over UDP. Does not cover the dogstatsd_socket (Unix socket) transport.
func (m *macosInstallSuite) TestDogstatsdMetricEndToEnd() {
	require.NoError(m.T(), m.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	m.macosPatchConfigAndRestart(macosTestClient, confFilePath, macosDogstatsdE2EMarker,
		"dd_url: "+fakeIntakeURL, "system/com.datadoghq.agent")

	const (
		gaugeMetric     = "e2e.macos.dogstatsd.gauge"
		gaugeTag        = "e2e:macos-gauge"
		countMetric     = "e2e.macos.dogstatsd.count"
		countTag        = "e2e:macos-count"
		histogramMetric = "e2e.macos.dogstatsd.histogram"
		histogramTag    = "e2e:macos-histogram"
		// histogram_aggregates defaults to include "count" (pkg/config/config_template.yaml),
		// so this suffix needs no extra config.
		histogramCountSuffix = ".count"
	)

	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`bash -c 'echo -n "%s:42|g|#%s" > /dev/udp/127.0.0.1/8125'`, gaugeMetric, gaugeTag,
	))
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`bash -c 'echo -n "%s:1|c|#%s" > /dev/udp/127.0.0.1/8125'`, countMetric, countTag,
	))
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`bash -c 'echo -n "%s:100|h|#%s" > /dev/udp/127.0.0.1/8125'`, histogramMetric, histogramTag,
	))

	// async delivery, so poll rather than assert once
	m.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := m.Env().FakeIntake.Client().FilterMetrics(gaugeMetric, client.WithTags[*aggregator.MetricSeries]([]string{gaugeTag}))
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEmpty(c, metrics, "%s tagged %q should be forwarded to fakeintake", gaugeMetric, gaugeTag)
	}, 2*time.Minute, 5*time.Second)

	m.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := m.Env().FakeIntake.Client().FilterMetrics(countMetric, client.WithTags[*aggregator.MetricSeries]([]string{countTag}))
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEmpty(c, metrics, "%s tagged %q should be forwarded to fakeintake", countMetric, countTag)
	}, 2*time.Minute, 5*time.Second)

	m.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := m.Env().FakeIntake.Client().FilterMetrics(histogramMetric+histogramCountSuffix, client.WithTags[*aggregator.MetricSeries]([]string{histogramTag}))
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEmpty(c, metrics, "%s tagged %q should be forwarded to fakeintake", histogramMetric+histogramCountSuffix, histogramTag)
	}, 2*time.Minute, 5*time.Second)
}

// TestProcessAgentReportsProcessData: macOS has no embedded process component
// (comp/process.Enabled() is false on darwin); instead a darwin-only corecheck
// (pkg/collector/corechecks/embed/process) spawns the standalone process-agent binary when
// process_collection.enabled is set. Enables that setting, starts a recognizable long-lived
// process, and asserts it appears in fakeintake's process payloads.
func (m *macosInstallSuite) TestProcessAgentReportsProcessData() {
	require.NoError(m.T(), m.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	// process check submits to process_config.process_dd_url, not the generic dd_url
	// (pkg/process/runner/endpoint/endpoints.go's GetAPIEndpoints), so set it explicitly
	m.macosPatchConfigAndRestart(macosTestClient, confFilePath, macosProcessAgentDataMarker,
		fmt.Sprintf("dd_url: %s\nprocess_config:\n  process_dd_url: %s\n  process_collection:\n    enabled: true", fakeIntakeURL, fakeIntakeURL),
		"system/com.datadoghq.agent")

	// registered before creation (mirrors macosPatchConfigAndRestart); launchctl remove is
	// idempotent even if submit below fails
	m.T().Cleanup(func() {
		if _, err := macosTestClient.Execute(fmt.Sprintf("sudo launchctl remove %s 2>/dev/null || true", macosProcessAgentSentinelProcess)); err != nil {
			m.T().Logf("cleanup: failed to remove sentinel launchd job %s: %v", macosProcessAgentSentinelProcess, err)
		}
	})

	// "sleep" isn't distinctive enough to search for, so symlink it under the sentinel name;
	// nohup over SSH proved unreliable, so launchd runs it instead
	macosTestClient.MustExecuteOn(m.T(), "ln -sf /bin/sleep /tmp/"+macosProcessAgentSentinelProcess)
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		"sudo launchctl submit -l %s -- /tmp/%s 300", macosProcessAgentSentinelProcess, macosProcessAgentSentinelProcess,
	))

	// async delivery; a process must be seen across two check runs before it's reported
	// (mirrors linux_test.go's TestProcessCheck)
	m.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := m.Env().FakeIntake.Client().GetProcesses()
		if !assert.NoError(c, err, "failed to get process payloads from fakeintake") {
			return
		}
		if !assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 process payloads received") {
			return
		}

		var found bool
		for _, payload := range payloads {
			for _, proc := range payload.Processes {
				if proc.Command == nil {
					continue
				}
				if strings.Contains(proc.Command.Comm, macosProcessAgentSentinelProcess) ||
					strings.Contains(proc.Command.Exe, macosProcessAgentSentinelProcess) ||
					(len(proc.Command.Args) > 0 && strings.Contains(proc.Command.Args[0], macosProcessAgentSentinelProcess)) {
					found = true
				}
			}
		}
		assert.True(c, found, "%s process should be collected in process payloads", macosProcessAgentSentinelProcess)
	}, 2*time.Minute, 10*time.Second)
}

// TestAPMTraceEndToEnd: macOS has no embedded trace-agent; a darwin-only corecheck
// (pkg/collector/corechecks/embed/apm) spawns the standalone trace-agent binary, enabled by
// default (no opt-in needed). Posts a trace directly to the receiver and asserts it reaches
// fakeintake's trace payloads.
func (m *macosInstallSuite) TestAPMTraceEndToEnd() {
	require.NoError(m.T(), m.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	// trace-agent submits to apm_config.apm_dd_url, not the generic dd_url
	// (pkg/config/setup/apm_settings.go), so set it explicitly
	m.macosPatchConfigAndRestart(macosTestClient, confFilePath, macosAPMTraceMarker,
		"apm_config:\n  apm_dd_url: "+fakeIntakeURL, "system/com.datadoghq.agent")

	// also wait for the trace receiver specifically -- it's a separate embedded HTTP server
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "curl -sf -o /dev/null http://127.0.0.1:8126/v0.4/traces -X POST -H 'Content-Type: application/json' -d '[]'")
	}, 20*time.Second, 1*time.Second)

	// Post a minimal trace directly to the receiver, tagged with a sentinel service name.
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(`curl -X POST http://127.0.0.1:8126/v0.4/traces \
-H 'X-Datadog-Trace-Count: 1' \
-H 'Content-Type: application/json' \
--data-binary @- <<EOF
[[{"trace_id":1234567890123456789,"span_id":9876543210987654321,"parent_id":0,"name":"http.request","resource":"GET /sentinel","service":"%s","type":"web","start":0,"duration":200000000,"meta":{"env":"e2e"},"metrics":{"_sampling_priority_v1":1}}]]
EOF`, macosAPMSentinelService))

	// async delivery, so poll rather than assert once
	m.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := m.Env().FakeIntake.Client().GetTraces()
		if !assert.NoError(c, err, "failed to get trace payloads from fakeintake") {
			return
		}

		var found bool
		for _, payload := range payloads {
			for _, tracerPayload := range payload.TracerPayloads {
				for _, chunk := range tracerPayload.Chunks {
					for _, span := range chunk.Spans {
						if span.Service == macosAPMSentinelService {
							found = true
						}
					}
				}
			}
		}
		assert.True(c, found, "%s trace should be collected in trace payloads", macosAPMSentinelService)
	}, 2*time.Minute, 10*time.Second)
}

// TestNPMTracesConnection opens a real connection to fakeintake's remote address and asserts
// it shows up in fakeintake's connection payloads.
func (m *macosInstallSuite) TestNPMTracesConnection() {
	require.NoError(m.T(), m.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	sysprobeConfFilePath := macosConfDefaultConfPath + "/system-probe.yaml"
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	parsedFakeIntakeURL, err := url.Parse(fakeIntakeURL)
	require.NoError(m.T(), err)
	fakeIntakeHost := parsedFakeIntakeURL.Hostname()
	fakeIntakePort, err := strconv.Atoi(parsedFakeIntakeURL.Port())
	require.NoError(m.T(), err)

	sysprobeBlock := "network_config:\n  enabled: true"
	processBlock := fmt.Sprintf("dd_url: %s\nprocess_config:\n  process_dd_url: %s\n  process_collection:\n    enabled: true", fakeIntakeURL, fakeIntakeURL)

	m.T().Cleanup(func() {
		m.macosRestartDaemon(macosTestClient, "system/com.datadoghq.sysprobe")
		m.macosRestartDaemon(macosTestClient, "system/com.datadoghq.agent")
		m.macosWaitForHealthyAgent(macosTestClient)
	})
	m.T().Cleanup(func() {
		m.macosRevertConfigBlock(macosTestClient, confFilePath, macosNPMProcessConfigMarker, processBlock)
	})
	m.T().Cleanup(func() {
		m.macosRevertConfigBlock(macosTestClient, sysprobeConfFilePath, macosNPMConfigMarker, sysprobeBlock)
	})

	m.macosAppendConfigBlock(macosTestClient, sysprobeConfFilePath, macosNPMConfigMarker, sysprobeBlock)
	m.macosAppendConfigBlock(macosTestClient, confFilePath, macosNPMProcessConfigMarker, processBlock)
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.sysprobe")
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")

	// Wait for sysprobe and the agent to come back healthy after the config changes.
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosLaunchdPID(c, macosTestClient, "system/com.datadoghq.sysprobe")
		macosTestClient.MustExecuteOn(c, macosAgentStatusCmd)
	}, 30*time.Second, 2*time.Second)

	// keep opening fresh connections while polling, since a one-shot connection could age out
	// before the next check run
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "curl -s -o /dev/null --max-time 2 "+fakeIntakeURL)

		payloads, err := m.Env().FakeIntake.Client().GetConnections()
		if !assert.NoError(c, err, "failed to get connection payloads from fakeintake") {
			return
		}

		var found bool
		payloads.ForeachHostnameConnections(func(cnx *aggregator.Connections, _ string) {
			for _, conn := range cnx.Connections {
				if conn.Raddr != nil && conn.Raddr.Ip == fakeIntakeHost && conn.Raddr.Port == int32(fakeIntakePort) {
					found = true
				}
			}
		})
		assert.True(c, found, "connection to fakeintake (%s:%d) should be collected in connection payloads", fakeIntakeHost, fakeIntakePort)
	}, 3*time.Minute, 10*time.Second)
}

// TestIntegrationsCommand exercises the `integration` command's Python/pip plumbing. Read-only
// and offline by design: install/remove hit the public TUF/pip CDN (see
// persisting_integrations_test.go's retry-backoff), so only freeze/show are covered here.
func (m *macosInstallSuite) TestIntegrationsCommand() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)

	// integration freeze lists datadog-* packages as "name==version"; sudo pip can print
	// unrelated warnings to the same stream, so only match freeze-shaped lines, and assert
	// datadog-checks-base specifically (a broken discovery would still print a non-empty list)
	freezeOutput, err := macosTestClient.Execute(macosAgentBinary + " integration freeze")
	assert.NoError(m.T(), err)
	freezeEntry := regexp.MustCompile(`^([a-zA-Z0-9._-]+)==`)
	var sawFreezeEntry, sawBasePackage bool
	for _, line := range strings.Split(freezeOutput, "\n") {
		matches := freezeEntry.FindStringSubmatch(strings.TrimSpace(line))
		if matches == nil {
			continue
		}
		sawFreezeEntry = true
		assert.True(m.T(), strings.HasPrefix(matches[1], "datadog-"), "unexpected non-datadog package in freeze output: %q", line)
		if matches[1] == macosBaseIntegrationPackage {
			sawBasePackage = true
		}
	}
	assert.True(m.T(), sawFreezeEntry, "integration freeze should list at least one package")
	assert.True(m.T(), sawBasePackage, "%s should be present in integration freeze output", macosBaseIntegrationPackage)

	// integration show: exercises the single-package read path independently of freeze.
	showOutput, err := macosTestClient.Execute(macosAgentBinary + " integration show " + macosBaseIntegrationPackage)
	assert.NoError(m.T(), err)
	assert.Regexp(m.T(), `\d+\.\d+\.\d+`, showOutput, "integration show should report a version for %s", macosBaseIntegrationPackage)
}

func (m *macosInstallSuite) TestAgentRestart() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)

	m.enableSysprobeForRestartTest(macosTestClient)
	m.testAgentRestart(macosTestClient)
}

// TestZZUninstallAgent runs cmd/agent/macos/uninstall_mac_os.sh against the suite's installed
// agent and asserts every service and file it manages is actually gone. The ZZ prefix makes
// this run last among this file's Test methods (tests run in alphabetical order within a
// suite), since the agent must stay installed for the others to use.
func (m *macosInstallSuite) TestZZUninstallAgent() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)

	_, thisFile, _, _ := runtime.Caller(0)
	localScriptPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..", "cmd", "agent", "macos", "uninstall_mac_os.sh")
	const remoteScriptPath = "/tmp/uninstall_mac_os.sh"

	m.Env().RemoteHost.CopyFile(localScriptPath, remoteScriptPath)
	macosTestClient.MustExecuteOn(m.T(), "chmod +x "+remoteScriptPath)
	macosTestClient.MustExecuteOn(m.T(), remoteScriptPath)

	for _, service := range []string{"com.datadoghq.agent", "com.datadoghq.sysprobe", "com.datadoghq.data-plane"} {
		_, err := macosTestClient.Execute("sudo launchctl print system/" + service)
		assert.Error(m.T(), err, "service %s should no longer be registered with launchd", service)
	}

	removedPaths := []string{
		"/Library/LaunchDaemons/com.datadoghq.agent.plist",
		"/Library/LaunchDaemons/com.datadoghq.sysprobe.plist",
		"/Library/LaunchDaemons/com.datadoghq.data-plane.plist",
		"/Library/LaunchAgents/com.datadoghq.gui.plist",
		"/Library/LaunchAgents/com.datadoghq.ai-usage-agent.desktop-monitor.plist",
		"/Library/LaunchAgents/com.datadoghq.ai-prompt-logger.desktop-monitor.plist",
		"/Applications/Datadog Agent.app",
		"/opt/datadog-agent",
		"/usr/local/bin/datadog-agent",
		"/var/log/datadog",
		"/private/var/root/datadog-install",
	}
	for _, path := range removedPaths {
		_, err := macosTestClient.Execute(fmt.Sprintf("sudo test -e %q", path))
		assert.Error(m.T(), err, "%s should have been removed by uninstall_mac_os.sh", path)
	}
}
