// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
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
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
)

const (
	macosAgentAPIPort  = 5001
	macosGUIPort       = 5002
	macosAuthTokenPath = "/opt/datadog-agent/etc/auth_token"
)

type macosInstallSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestMacosInstallScript(t *testing.T) {
	extraConfigMap := runner.ConfigMap{}
	// When the environment is initialized Pulumi needs to be aware that it must chose in a smaller subset of subnet on MacOS.
	// Going directly through the configmap is the only way we have for now to let Pulumi know about it.
	extraConfigMap.Set("ddinfra:aws/useMacosCompatibleSubnets", "true", false)
	e2e.Run(t, &macosInstallSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoAgentNoFakeIntake(
			awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.MacOSDefault))),
			awshost.WithExtraConfigParams(extraConfigMap),
		)),
	)
}

// SetupSuite installs the agent once before any of the suite's Test methods run,
// so TestInstallAgent and TestAgentRestart can each assert independently against
// the same already-installed environment instead of one depending on the other.
func (m *macosInstallSuite) SetupSuite() {
	m.BaseSuite.SetupSuite()

	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	install.MacOS(m.T(), macosTestClient, installparams.WithUsername(m.Env().RemoteHost.Username), installparams.WithArch("x64"))

	// The agent should start at some point
	m.EventuallyWithT(func(c *assert.CollectT) {
		macosTestClient.MustExecuteOn(c, "sudo /usr/local/bin/datadog-agent status")
	}, 20*time.Second, 1*time.Second)
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
