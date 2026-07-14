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
	macosAgentAPIPort        = 5001
	macosGUIPort             = 5002
	macosAuthTokenPath       = "/opt/datadog-agent/etc/auth_token"
	macosConfDefaultConfPath = "/opt/datadog-agent/etc"
)

// macosSharedStackName pins every macOS E2E suite in this package to the same
// Pulumi stack/EC2 host instead of each suite type spawning its own instance
// (by default, the stack name is derived per Go type, see suite.go's
// e2e-<SuiteTypeName>-<hash> naming). Any new macOS suite added to this
// package should pass e2e.WithStackName(macosSharedStackName) and
// e2e.WithDevMode() in its entry-point function so it keeps targeting this
// same shared host rather than provisioning a new one.
const macosSharedStackName = "e2e-macosInstallSuite-d46bf3fab209fab6"

type macosInstallSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestMacosInstallScript(t *testing.T) {
	extraConfigMap := runner.ConfigMap{}
	// When the environment is initialized Pulumi needs to be aware that it must chose in a smaller subset of subnet on MacOS.
	// Going directly through the configmap is the only way we have for now to let Pulumi know about it.
	extraConfigMap.Set("ddinfra:aws/useMacosCompatibleSubnets", "true", false)
	e2e.Run(t, &macosInstallSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.MacOSDefault)), ec2.WithoutAgent()),
			awshost.WithExtraConfigParams(extraConfigMap),
		)),
		e2e.WithStackName(macosSharedStackName),
		e2e.WithDevMode(),
	)
}

// SetupSuite installs the agent once before any of the suite's Test methods run,
// so TestInstallAgent and TestAgentRestart can each assert independently against
// the same already-installed environment instead of one depending on the other.
//
// The suite reuses a shared/devmode host (see macosSharedStackName) across runs, so a
// prior run's agent build can still be resident here. removePreInstalledAgent wipes it
// first so a failed reinstall can never be masked by that stale leftover.
func (m *macosInstallSuite) SetupSuite() {
	m.BaseSuite.SetupSuite()

	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	removePreInstalledAgent(macosTestClient)

	install.MacOS(m.T(), macosTestClient, installparams.WithUsername(m.Env().RemoteHost.Username), installparams.WithArch("x64"))
	if m.T().Failed() {
		m.FailNow("agent install failed, aborting SetupSuite")
	}

	// The agent should start at some point
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
	}, 20*time.Second, 1*time.Second)
}

// removePreInstalledAgent tears down any agent already present on the host, mirroring the
// system-wide cleanup in cmd/agent/macos/uninstall_mac_os.sh. It is a no-op (all commands
// are best-effort) when no agent is installed.
func removePreInstalledAgent(client *common.MacOSTestClient) {
	cmd := `
sudo launchctl bootout system/com.datadoghq.agent 2>/dev/null || true
sudo launchctl bootout system/com.datadoghq.sysprobe 2>/dev/null || true
sudo launchctl bootout system/com.datadoghq.data-plane 2>/dev/null || true
sudo rm -f /Library/LaunchDaemons/com.datadoghq.agent.plist
sudo rm -f /Library/LaunchDaemons/com.datadoghq.sysprobe.plist
sudo rm -f /Library/LaunchDaemons/com.datadoghq.data-plane.plist
sudo rm -rf "/Applications/Datadog Agent.app"
sudo rm -rf /opt/datadog-agent
sudo rm -f /usr/local/bin/datadog-agent
sudo rm -f /var/log/datadog
`
	_, _ = client.Execute(cmd)
}

func (m *macosInstallSuite) TestInstallAgent() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)

	_, err := macosTestClient.Execute("sudo test -x /opt/datadog-agent/embedded/bin/agent-data-plane")
	assert.NoError(m.T(), err)
	_, err = macosTestClient.Execute("sudo test -f /Library/LaunchDaemons/com.datadoghq.data-plane.plist")
	assert.NoError(m.T(), err)
	_, err = macosTestClient.Execute("sudo launchctl print system/com.datadoghq.data-plane")
	assert.NoError(m.T(), err)

	// check that there is no world-writable files or directories in /opt/datadog-agent
	// exclude /opt/datadog-agent/run/ipc which is intentionally world-writable for multi-user GUI sockets
	worldWritableFiles, err := macosTestClient.Execute("sudo find /opt/datadog-agent \\( -type f -o -type d \\) -perm -002 ! -path '/opt/datadog-agent/run/ipc' ! -path '/opt/datadog-agent/run/ipc/*'")
	assert.NoError(m.T(), err)
	assert.Empty(m.T(), strings.TrimSpace(worldWritableFiles))
}

// macosStatusAndConfigSanityTag is a distinctive value round-tripped through the config
// file, the running agent's runtime config, and its status output, to prove the full
// config-reload pipeline works end to end rather than just checking commands don't error.
const macosStatusAndConfigSanityTag = "e2e-sanity:macos"

// macosStatusAndConfigMarker delimits the block TestAgentStatusAndConfig appends to
// datadog.yaml, so it can be identified and removed again during cleanup.
const macosStatusAndConfigMarker = "# added by e2e TestAgentStatusAndConfig"

func (m *macosInstallSuite) TestAgentStatusAndConfig() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"

	// Set a distinctive, verifiable config value and reload the agent to pick it up.
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q %s || printf '\n%s\ntags:\n  - %s\n' | sudo tee -a %s`,
		macosStatusAndConfigMarker, confFilePath, macosStatusAndConfigMarker, macosStatusAndConfigSanityTag, confFilePath,
	))
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")

	m.T().Cleanup(func() {
		macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
			`sudo sed -i '' "/%s/,+2d" %s`, macosStatusAndConfigMarker, confFilePath,
		))
		macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")
		m.EventuallyWithT(func(c *assert.CollectT) {
			macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
		}, 20*time.Second, 1*time.Second)
	})

	// Wait for the agent to come back healthy after the config change.
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
	}, 20*time.Second, 1*time.Second)

	// Status: functional signals, not just "the command didn't error".
	statusOutput, err := macosTestClient.Execute("sudo /usr/local/bin/datadog-agent status")
	assert.NoError(m.T(), err)
	statusOutput = common.SanitizeStatusOutputForKnownNoise(statusOutput)
	assert.NotContains(m.T(), statusOutput, "ERROR")
	assert.Contains(m.T(), statusOutput, "Forwarder")
	assert.Contains(m.T(), statusOutput, "Host Info")
	assert.Contains(m.T(), statusOutput, "DogStatsD")
	assert.Contains(m.T(), statusOutput, macosStatusAndConfigSanityTag)

	// Checks are actually scheduled/running, not just that the status command ran.
	// Right after the restart above, the first check run cycle may not have completed
	// yet, so poll instead of asserting once (mirrors CheckAgentBehaviour on Linux/Windows).
	m.EventuallyWithT(func(c *assert.CollectT) {
		jsonStatus, err := macosTestClient.Execute("sudo /usr/local/bin/datadog-agent status -j")
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

	// agent config get/set: exercises the runtime settings API directly. tags isn't a
	// registered runtime setting (only specific settings like log_level are gettable via
	// `agent config get`), so use log_level for this round trip instead.
	m.T().Cleanup(func() {
		macosTestClient.MustExecuteOn(m.T(), "sudo /usr/local/bin/datadog-agent config set log_level info")
	})
	_, err = macosTestClient.Execute("sudo /usr/local/bin/datadog-agent config set log_level debug")
	assert.NoError(m.T(), err)
	logLevelOutput, err := macosTestClient.Execute("sudo /usr/local/bin/datadog-agent config get log_level")
	assert.NoError(m.T(), err)
	assert.Contains(m.T(), logLevelOutput, "debug")

	// agent version: content check, not just exit code.
	versionOutput, err := macosTestClient.Execute("sudo /usr/local/bin/datadog-agent version")
	assert.NoError(m.T(), err)
	assert.Regexp(m.T(), `Agent \d+\.\d+\.\d+`, versionOutput)
}

// macosEssentialChecks are the core checks a default macOS install always schedules,
// regardless of container/cloud/Kubernetes context (verified by polling a fresh
// install for 90s: this set stabilizes by ~t=35s and stays constant afterward).
// They back the host's core metrics (system.cpu.*, system.mem.*, system.disk.*,
// system.net.*, system.load.*, system.uptime, ntp.offset); losing any of them would
// leave the agent reporting healthy status while silently missing whole metric
// families. Checks that ship a conf.yaml.default but depend on hardware (battery,
// wlan) or a runtime context (containerd, cri, kubelet, ecs_fargate, ...) are
// intentionally excluded, since they legitimately don't run on a bare EC2 host.
var macosEssentialChecks = []string{
	"cloud_hostinfo", "container_image", "container_lifecycle",
	"cpu", "disk", "io", "load", "memory", "network", "ntp", "telemetry", "uptime",
}

// TestEssentialChecksLoaded asserts that the checks backing the agent's core host
// metrics are actually scheduled and running, not just that some check runs (the
// generic non-empty assertion in TestAgentStatusAndConfig would still pass if a
// build regressed default-check registration and dropped cpu/memory/disk/network/ntp
// entirely). It runs read-only against the state SetupSuite already installed.
func (m *macosInstallSuite) TestEssentialChecksLoaded() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)

	// Check scheduling is staggered on startup, so poll rather than assert once
	// (mirrors the check-running poll in TestAgentStatusAndConfig).
	m.EventuallyWithT(func(c *assert.CollectT) {
		jsonStatus, err := macosTestClient.Execute("sudo /usr/local/bin/datadog-agent status -j")
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

// macosCPUMetricsMarker delimits the block TestCpuReportsSignalMetrics appends to
// datadog.yaml, so it can be identified and removed again during cleanup.
const macosCPUMetricsMarker = "# added by e2e TestCpuReportsSignalMetrics"

// TestCpuReportsSignalMetrics proves the cpu check doesn't just get scheduled
// (TestEssentialChecksLoaded) but actually collects and successfully forwards real
// metric data. It redirects the already-running agent's dd_url at this suite's
// fakeintake and asserts a cpu metric shows up there, which is the only way to
// distinguish "the check runs" from "the check runs and its data reaches Datadog".
func (m *macosInstallSuite) TestCpuReportsSignalMetrics() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q %s || printf '\n%s\ndd_url: %s\n' | sudo tee -a %s`,
		macosCPUMetricsMarker, confFilePath, macosCPUMetricsMarker, fakeIntakeURL, confFilePath,
	))
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")

	m.T().Cleanup(func() {
		macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
			`sudo sed -i '' "/%s/,+1d" %s`, macosCPUMetricsMarker, confFilePath,
		))
		macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")
		m.EventuallyWithT(func(c *assert.CollectT) {
			macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
		}, 20*time.Second, 1*time.Second)
	})

	// Wait for the agent to come back healthy after redirecting dd_url.
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
	}, 20*time.Second, 1*time.Second)

	// Delivery is async (collection interval + forwarder flush), so poll fakeintake
	// rather than asserting once.
	m.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := m.Env().FakeIntake.Client().FilterMetrics("system.cpu.idle")
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEmpty(c, metrics, "system.cpu.idle should be forwarded to fakeintake")
	}, 2*time.Minute, 5*time.Second)
}

// macosDogstatsdMarker delimits the block TestDogstatsdListening appends to
// datadog.yaml, so it can be identified and removed again during cleanup.
const macosDogstatsdMarker = "# added by e2e TestDogstatsdListening"

// TestDogstatsdListening proves the agent's embedded DogStatsD UDP listener is not just
// bound to 127.0.0.1:8125, but actually receives, aggregates, and forwards a real metric.
// A "port is open" check alone wouldn't catch a broken parser, aggregator, or forwarder.
func (m *macosInstallSuite) TestDogstatsdListening() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	boundPorts := macosTestClient.MustExecuteOn(m.T(), "sudo lsof -nP -iUDP:8125")
	assert.Contains(m.T(), boundPorts, "agent", "the agent process should be bound to UDP 8125")

	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q %s || printf '\n%s\ndd_url: %s\n' | sudo tee -a %s`,
		macosDogstatsdMarker, confFilePath, macosDogstatsdMarker, fakeIntakeURL, confFilePath,
	))
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")

	m.T().Cleanup(func() {
		macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
			`sudo sed -i '' "/%s/,+1d" %s`, macosDogstatsdMarker, confFilePath,
		))
		macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")
		m.EventuallyWithT(func(c *assert.CollectT) {
			macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
		}, 20*time.Second, 1*time.Second)
	})

	// Wait for the agent to come back healthy after redirecting dd_url.
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
	}, 20*time.Second, 1*time.Second)

	const metricName = "e2e.macos.dogstatsd.sanity"
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`bash -c 'echo -n "%s:1|c" > /dev/udp/127.0.0.1/8125'`, metricName,
	))

	// Delivery is async (aggregation flush + forwarder flush), so poll fakeintake
	// rather than asserting once.
	m.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := m.Env().FakeIntake.Client().FilterMetrics(metricName)
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEmpty(c, metrics, "%s should be forwarded to fakeintake", metricName)
	}, 2*time.Minute, 5*time.Second)
}

// macosDogstatsdE2EMarker delimits the block TestDogstatsdMetricEndToEnd appends to
// datadog.yaml, so it can be identified and removed again during cleanup.
const macosDogstatsdE2EMarker = "# added by e2e TestDogstatsdMetricEndToEnd"

// TestDogstatsdMetricEndToEnd broadens TestDogstatsdListening's single-count-metric check
// into coverage across DogStatsD's metric types (gauge, count, histogram) and tag
// propagation over UDP. It does not cover the dogstatsd_socket (Unix socket) transport,
// which is a separate, untested code path.
func (m *macosInstallSuite) TestDogstatsdMetricEndToEnd() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q %s || printf '\n%s\ndd_url: %s\n' | sudo tee -a %s`,
		macosDogstatsdE2EMarker, confFilePath, macosDogstatsdE2EMarker, fakeIntakeURL, confFilePath,
	))
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")

	m.T().Cleanup(func() {
		macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
			`sudo sed -i '' "/%s/,+1d" %s`, macosDogstatsdE2EMarker, confFilePath,
		))
		macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")
		m.EventuallyWithT(func(c *assert.CollectT) {
			macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
		}, 20*time.Second, 1*time.Second)
	})

	// Wait for the agent to come back healthy after redirecting dd_url.
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
	}, 20*time.Second, 1*time.Second)

	const (
		gaugeMetric     = "e2e.macos.dogstatsd.gauge"
		gaugeTag        = "e2e:macos-gauge"
		countMetric     = "e2e.macos.dogstatsd.count"
		countTag        = "e2e:macos-count"
		histogramMetric = "e2e.macos.dogstatsd.histogram"
		histogramTag    = "e2e:macos-histogram"
		// histogram_aggregates defaults to ["max", "median", "avg", "count"] (see
		// pkg/config/config_template.yaml), so the agent flushes this suffix without
		// any extra config.
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

	// Delivery is async (aggregation flush + forwarder flush), so poll fakeintake
	// rather than asserting once.
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

// macosProcessAgentDataMarker delimits the block TestProcessAgentReportsProcessData appends
// to datadog.yaml, so it can be identified and removed again during cleanup.
const macosProcessAgentDataMarker = "# added by e2e TestProcessAgentReportsProcessData"

// macosProcessAgentSentinelProcess is a distinctive, long-lived process started by this test
// so it can be searched for by name in the process payloads collected by fakeintake, rather
// than asserting on "any process data arrived" which could pass even if collection were broken
// for every real process but happened to report stale/empty data for something else.
const macosProcessAgentSentinelProcess = "ddprocsentinel"

// TestProcessAgentReportsProcessData documents macOS's process-collection architecture, which is
// unlike Linux/Windows: there is no embedded process component (comp/process's Enabled() is
// hardcoded false on darwin, see comp/process/agent/agent_fallback.go). Instead, macOS ships a
// darwin-only corecheck (pkg/collector/corechecks/embed/process) that spawns the bundled
// standalone process-agent binary once process_config.process_collection.enabled is set. It's
// not enough for that check to be scheduled, its collected data must actually reach the backend.
// It enables process_config.process_collection.enabled (off by default), starts a
// recognizable long-lived process, and asserts that process shows up in the process
// payloads fakeintake receives.
func (m *macosInstallSuite) TestProcessAgentReportsProcessData() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	// The process check submits to process_config.process_dd_url (falling back to the "site"
	// default), not the generic dd_url used by metrics/logs -- so it must be set explicitly to
	// point at fakeintake (see pkg/process/runner/endpoint/endpoints.go's GetAPIEndpoints).
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q %s || printf '\n%s\ndd_url: %s\nprocess_config:\n  process_dd_url: %s\n  process_collection:\n    enabled: true\n' | sudo tee -a %s`,
		macosProcessAgentDataMarker, confFilePath, macosProcessAgentDataMarker, fakeIntakeURL, fakeIntakeURL, confFilePath,
	))
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")

	// "sleep" itself wouldn't be a distinctive enough name to search for in process payloads,
	// so run it through a symlink named after the sentinel constant instead. `nohup ... &`
	// over a non-interactive SSH exec proved unreliable (the backgrounded job never actually
	// showed up), so hand it to launchd instead, which is designed to keep a job running
	// fully independent of the SSH session that submitted it.
	macosTestClient.MustExecuteOn(m.T(), "ln -sf /bin/sleep /tmp/"+macosProcessAgentSentinelProcess)
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		"sudo launchctl submit -l %s -- /tmp/%s 300", macosProcessAgentSentinelProcess, macosProcessAgentSentinelProcess,
	))

	m.T().Cleanup(func() {
		macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf("sudo launchctl remove %s 2>/dev/null || true", macosProcessAgentSentinelProcess))
		macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
			`sudo sed -i '' "/%s/,+5d" %s`, macosProcessAgentDataMarker, confFilePath,
		))
		macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")
		m.EventuallyWithT(func(c *assert.CollectT) {
			macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
		}, 20*time.Second, 1*time.Second)
	})

	// Wait for the agent to come back healthy after enabling process collection.
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
	}, 20*time.Second, 1*time.Second)

	// Delivery is async (collection interval + forwarder flush), and a process must be seen
	// across two check runs before it's reported, so poll rather than assert once. This mirrors
	// the Linux precedent in test/new-e2e/tests/process/linux_test.go's TestProcessCheck.
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

// macosAPMTraceMarker delimits the block TestAPMTraceEndToEnd appends to
// datadog.yaml, so it can be identified and removed again during cleanup.
const macosAPMTraceMarker = "# added by e2e TestAPMTraceEndToEnd"

// macosAPMSentinelService is a distinctive service name attached to the test trace, so it
// can be searched for in the trace payloads collected by fakeintake, rather than asserting on
// "any trace arrived" which could pass even if it came from something else entirely.
const macosAPMSentinelService = "ddapmsentinel"

// TestAPMTraceEndToEnd documents macOS's trace-collection architecture, which is unlike
// Linux/Windows: there is no embedded trace-agent component either. macOS ships a darwin-only
// corecheck (pkg/collector/corechecks/embed/apm) that spawns the bundled standalone trace-agent
// binary, which is enabled and listening on the receiver port by default (unlike process
// collection, there's no explicit opt-in). That default-enabled receiver being reachable
// wouldn't prove data actually reaches the backend though, so this test posts a trace directly
// to the receiver and asserts it shows up in the trace payloads fakeintake receives.
func (m *macosInstallSuite) TestAPMTraceEndToEnd() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	// The trace-agent submits to apm_config.apm_dd_url (falling back to the "site" default),
	// not the generic dd_url used by metrics/logs -- so it must be set explicitly to point at
	// fakeintake (see pkg/config/setup/apm_settings.go).
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q %s || printf '\n%s\napm_config:\n  apm_dd_url: %s\n' | sudo tee -a %s`,
		macosAPMTraceMarker, confFilePath, macosAPMTraceMarker, fakeIntakeURL, confFilePath,
	))
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")

	m.T().Cleanup(func() {
		macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
			`sudo sed -i '' "/%s/,+2d" %s`, macosAPMTraceMarker, confFilePath,
		))
		macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")
		m.EventuallyWithT(func(c *assert.CollectT) {
			macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
		}, 20*time.Second, 1*time.Second)
	})

	// Wait for the agent to come back healthy, and the receiver to be listening, after
	// redirecting apm_dd_url.
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
		macosTestClient.MustExecuteOn(c, "curl -sf -o /dev/null http://127.0.0.1:8126/v0.4/traces -X POST -H 'Content-Type: application/json' -d '[]'")
	}, 20*time.Second, 1*time.Second)

	// Post a minimal trace directly to the receiver, tagged with a sentinel service name.
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(`curl -X POST http://127.0.0.1:8126/v0.4/traces \
-H 'X-Datadog-Trace-Count: 1' \
-H 'Content-Type: application/json' \
--data-binary @- <<EOF
[[{"trace_id":1234567890123456789,"span_id":9876543210987654321,"parent_id":0,"name":"http.request","resource":"GET /sentinel","service":"%s","type":"web","start":0,"duration":200000000,"meta":{"env":"e2e"},"metrics":{"_sampling_priority_v1":1}}]]
EOF`, macosAPMSentinelService))

	// Delivery is async (trace-agent flush interval + forwarder flush), so poll rather than
	// assert once.
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

// macosNPMConfigMarker delimits the network_config block TestNPMTracesConnection appends to
// system-probe.yaml, so it can be identified and removed again during cleanup.
const macosNPMConfigMarker = "# added by e2e TestNPMTracesConnection"

// macosNPMProcessConfigMarker delimits the block TestNPMTracesConnection appends to
// datadog.yaml. The connections check runs inside the same standalone process-agent binary
// spawned for process collection (see TestProcessAgentReportsProcessData) and submits over
// the same process_config.process_dd_url, so that setup is duplicated here rather than shared,
// to keep this test independent of whether TestProcessAgentReportsProcessData already ran.
const macosNPMProcessConfigMarker = "# added by e2e TestNPMTracesConnection process config"

// TestNPMTracesConnection documents macOS's NPM architecture, which is unlike Linux: there is
// no eBPF on Darwin, so the darwin-only NetworkTracer module
// (cmd/system-probe/modules/network_tracer_darwin.go) always falls back to a libpcap-based,
// ebpf-less connection tracer (pkg/network/tracer/connection/tracer_darwin.go) that has no
// per-socket process attribution (unlike Linux, a captured connection's PID is always zero on
// Darwin). It also never captures loopback traffic at all: isEligibleInterface explicitly
// skips any interface with net.FlagLoopback (pkg/network/filter/packet_source_darwin.go), so a
// connection to 127.0.0.1 is invisible to it. Enabling network_config.enabled isn't enough to
// prove connections actually reach the backend anyway, so this test opens a real connection to
// fakeintake's own remote address -- guaranteed to cross a real, captured interface -- and
// asserts it shows up in the connection payloads fakeintake receives.
func (m *macosInstallSuite) TestNPMTracesConnection() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	sysprobeConfFilePath := macosConfDefaultConfPath + "/system-probe.yaml"
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	fakeIntakeURL := m.Env().FakeIntake.URL

	parsedFakeIntakeURL, err := url.Parse(fakeIntakeURL)
	require.NoError(m.T(), err)
	fakeIntakeHost := parsedFakeIntakeURL.Hostname()
	fakeIntakePort, err := strconv.Atoi(parsedFakeIntakeURL.Port())
	require.NoError(m.T(), err)

	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q %s || printf '\n%s\nnetwork_config:\n  enabled: true\n' | sudo tee -a %s`,
		macosNPMConfigMarker, sysprobeConfFilePath, macosNPMConfigMarker, sysprobeConfFilePath,
	))
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q %s || printf '\n%s\ndd_url: %s\nprocess_config:\n  process_dd_url: %s\n  process_collection:\n    enabled: true\n' | sudo tee -a %s`,
		macosNPMProcessConfigMarker, confFilePath, macosNPMProcessConfigMarker, fakeIntakeURL, fakeIntakeURL, confFilePath,
	))
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.sysprobe")
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")

	m.T().Cleanup(func() {
		macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
			`sudo sed -i '' "/%s/,+2d" %s`, macosNPMConfigMarker, sysprobeConfFilePath,
		))
		macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
			`sudo sed -i '' "/%s/,+5d" %s`, macosNPMProcessConfigMarker, confFilePath,
		))
		macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.sysprobe")
		macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.agent")
		m.EventuallyWithT(func(c *assert.CollectT) {
			macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
		}, 20*time.Second, 1*time.Second)
	})

	// Wait for sysprobe and the agent to come back healthy after the config changes.
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosLaunchdPID(c, macosTestClient, "system/com.datadoghq.sysprobe")
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
	}, 30*time.Second, 2*time.Second)

	// A one-shot connection could close and age out of the tracer's state before the
	// connections check next polls it, so keep opening fresh connections while polling
	// fakeintake rather than connecting once up front.
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

func (m *macosInstallSuite) TestAgentRestart() {
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)

	m.enableSysprobeForRestartTest(macosTestClient)
	m.testAgentRestart(macosTestClient)
}

// enableSysprobeForRestartTest turns on a lightweight system-probe module and
// kickstarts the daemon. On a default install every module ships disabled, so
// system-probe exits immediately and stays down (launchd won't relaunch it after
// a clean exit); the GUI's /agent-restart handler lives inside system-probe's own
// HTTP API, so it needs to actually be running for testAgentRestart to exercise it.
func (m *macosInstallSuite) enableSysprobeForRestartTest(macosTestClient *common.MacOSTestClient) {
	// Enable traceroute
	const marker = "# added by e2e testAgentRestart"
	macosTestClient.MustExecuteOn(m.T(), fmt.Sprintf(
		`sudo grep -qF %q /opt/datadog-agent/etc/system-probe.yaml || printf '\n%s\ntraceroute:\n  enabled: true\n' | sudo tee -a /opt/datadog-agent/etc/system-probe.yaml`,
		marker, marker,
	))
	macosTestClient.MustExecuteOn(m.T(), "sudo launchctl kickstart -k system/com.datadoghq.sysprobe")

	m.EventuallyWithT(func(c *assert.CollectT) {
		macosLaunchdPID(c, macosTestClient, "system/com.datadoghq.sysprobe")
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
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
	}, 20*time.Second, 1*time.Second)
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
