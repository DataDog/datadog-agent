// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

//go:embed fixtures/ddinjector_crash.ps1
var runDDInjectorCrashScript string

type crashProcessResult struct {
	ProcessID  uint32 `json:"process_id"`
	ExitStatus string `json:"exit_status"`
}

type testInjectorCrashTelemetry struct {
	baseAPMInjectSuite
}

// TestInjectorCrashTelemetry verifies the deployed Agent's DDInjector ETW-to-telemetry path.
func TestInjectorCrashTelemetry(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &testInjectorCrashTelemetry{},
		e2e.WithProvisioner(winawshost.ProvisionerNoAgent()))
}

func (s *testInjectorCrashTelemetry) AfterTest(suiteName, testName string) {
	s.baseAPMInjectSuite.AfterTest(suiteName, testName)
	_, err := s.Env().RemoteHost.Execute(`
Remove-Item -Path "HKLM:\SOFTWARE\Microsoft\Windows\Windows Error Reporting\LocalDumps\ddinjector-e2e-crash.exe" -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item -Path "C:\ddinjector-e2e" -Recurse -Force -ErrorAction SilentlyContinue
`)
	s.Assert().NoError(err, "should clean up the DDInjector crash fixture")
	s.Installer().Purge()
}

func (s *testInjectorCrashTelemetry) TestCrashEventReachesAgentTelemetry() {
	s.installCurrentAgentVersionWithAPMInject(
		WithExtraEnvVars(map[string]string{
			"DD_APM_INSTRUMENTATION_ENABLED":                      "host",
			"DD_INSTALLER_REGISTRY_URL":                           "install.datad0g.com",
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT": s.currentAPMInjectVersion.PackageVersion(),
			"DD_APM_INSTRUMENTATION_LIBRARIES":                    "dotnet:3",
		}),
	)
	s.assertSuccessfulPromoteExperiment()
	s.enableInjectorTelemetry()
	s.configureAgentTelemetryIntake()
	s.assertDriverInjections(true)
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	output := s.Env().RemoteHost.MustExecute(runDDInjectorCrashScript)
	var process crashProcessResult
	require.NoError(s.T(), json.Unmarshal([]byte(strings.TrimSpace(output)), &process), "invalid crash fixture output: %s", output)
	require.NotZero(s.T(), process.ProcessID)
	exitStatus, err := strconv.ParseUint(strings.TrimPrefix(process.ExitStatus, "0x"), 16, 32)
	require.NoError(s.T(), err, "invalid process exit status %q", process.ExitStatus)
	require.Equal(s.T(), uint64(0xc0000000), exitStatus&0xc0000000, "the fixture process should terminate with an NT error")

	s.EventuallyWithT(func(c *assert.CollectT) {
		crashes, err := s.Env().FakeIntake.Client().GetDDInjectorCrashes()
		require.NoError(c, err)
		for _, crash := range crashes {
			if crash.ProcessID != process.ProcessID {
				continue
			}
			assert.Equal(c, "ddinjector-e2e-crash.exe", crash.ProcessName)
			assert.Equal(c, process.ExitStatus, crash.ExitStatus)
			assert.GreaterOrEqual(c, crash.ElapsedMs, int64(0))
			assert.LessOrEqual(c, crash.ElapsedMs, int64(1000))
			assert.Contains(c, []string{"during_injection", "post_injection"}, crash.Phase)
			return
		}
		assert.Fail(c, "DDInjector crash telemetry not found", "no event for PID %d among %d crash events", process.ProcessID, len(crashes))
	}, 2*time.Minute, 10*time.Second)

	stats := s.queryInjectorStats(true)
	duringInjection, ok := stats["crashes_during_injection"].(float64)
	s.Require().True(ok, "crashes_during_injection should be a number: %+v", stats)
	postInjection, ok := stats["crashes_post_injection"].(float64)
	s.Require().True(ok, "crashes_post_injection should be a number: %+v", stats)
	s.Require().Greater(duringInjection+postInjection, float64(0), "the DDInjector driver should report an attributed crash")
}

func (s *testInjectorCrashTelemetry) configureAgentTelemetryIntake() {
	host := s.Env().RemoteHost
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(host)
	s.Require().NoError(err)
	configPath := filepath.Join(configRoot, "datadog.yaml")
	config, err := s.readYamlConfig(configPath)
	s.Require().NoError(err)

	fakeIntakeURL, err := url.Parse(s.Env().FakeIntake.URL)
	s.Require().NoError(err)
	s.Require().NotEmpty(fakeIntakeURL.Host)
	config["agent_telemetry"] = map[string]interface{}{
		"enabled":         true,
		"logs_dd_url":     fakeIntakeURL.Host,
		"logs_no_ssl":     fakeIntakeURL.Scheme == "http",
		"use_compression": false,
	}
	s.Require().NoError(s.writeYamlConfig(configPath, config))

	s.Require().NoError(windowscommon.RestartService(host, "datadogagent"))
	s.Require().NoError(s.WaitForAgentService("Running"))
	s.T().Logf("Agent telemetry configured for %s://%s", fakeIntakeURL.Scheme, fakeIntakeURL.Host)
}
