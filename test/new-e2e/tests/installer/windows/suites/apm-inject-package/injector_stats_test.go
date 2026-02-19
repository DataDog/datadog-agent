// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package injecttests contains E2E tests for the Injector package.
// This file tests the injector statistics reporting functionality through system-probe.
package injecttests

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type testInjectorStats struct {
	baseSuite
}

// TestInjectorStats tests querying injector statistics via system-probe
func TestInjectorStats(t *testing.T) {
	e2e.Run(t, &testInjectorStats{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testInjectorStats) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.baseSuite.AfterTest(suiteName, testName)
}

// TestQueryStatsViaSystemProbe tests querying injector stats through system-probe HTTP endpoint
func (s *testInjectorStats) TestQueryStatsViaSystemProbe() {
	// Install agent with APM inject enabled
	s.installCurrentAgentVersionWithAPMInject(
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_APM_INSTRUMENTATION_ENABLED": "host",
			// TODO: remove override once image is published in prod
			"DD_INSTALLER_REGISTRY_URL":                           "install.datad0g.com",
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT": s.currentAPMInjectVersion.PackageVersion(),
			"DD_APM_INSTRUMENTATION_LIBRARIES":                    "dotnet:3",
		}),
	)

	// Verify the package is installed
	s.assertSuccessfulPromoteExperiment()

	// Explicitly enable injector telemetry in system-probe
	s.enableInjectorTelemetry()

	s.waitForServiceRunning()

	stats := s.queryInjectorStats(false)
	s.verifyStatsStructure(stats)
}

// TestQueryStatsAfterInjection tests that stats are updated after actual injection occurs
func (s *testInjectorStats) TestQueryStatsAfterInjection() {
	// Install agent with APM inject enabled
	s.installCurrentAgentVersionWithAPMInject(
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_APM_INSTRUMENTATION_ENABLED": "host",
			// TODO: remove override once image is published in prod
			"DD_INSTALLER_REGISTRY_URL":                           "install.datad0g.com",
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT": s.currentAPMInjectVersion.PackageVersion(),
			"DD_APM_INSTRUMENTATION_LIBRARIES":                    "dotnet:3",
		}),
	)

	// Verify the package is installed
	s.assertSuccessfulPromoteExperiment()

	// Explicitly enable injector telemetry in system-probe
	s.enableInjectorTelemetry()

	s.waitForServiceRunning()

	// Get initial stats
	initialStats := s.queryInjectorStats(true)
	s.verifyStatsStructure(initialStats)

	// Trigger injection by running a process
	s.assertDriverInjections(true)

	// Wait a bit for stats to update
	time.Sleep(5 * time.Second)

	// Get updated stats
	updatedStats := s.queryInjectorStats(true)
	s.verifyStatsStructure(updatedStats)

	// Verify that we did trigger the Collect callback and got back stats.
	// If last_check_timestamp is 0, check whether ddinjector is running, check system-probe.yaml,
	// and check the telemetry scheduler's delay start amount.
	ts, ok := updatedStats["last_check_timestamp"].(float64)
	s.Require().True(ok, "last_check_timestamp should be in stats: %+v", updatedStats)
	if ts == 0 {
		s.logInjectorDiagnostics()
		s.Require().True(ts > 0, "stats did not refresh")
	}

	// Verify that some counters have increased (at least one injection should have occurred)
	// Note: We can't guarantee specific counters will increase in all test environments,
	// but we can verify the stats endpoint is working and returning valid data
	s.T().Logf("Initial stats: %+v", initialStats)
	s.T().Logf("Updated stats: %+v", updatedStats)

	diffFound := false
	for k, v1 := range initialStats {
		v2, ok := updatedStats[k]
		s.Require().True(ok, "updated injector stats should have key: %s", k)
		if v1.(float64) != v2.(float64) {
			diffFound = true
			break
		}
	}

	s.Assert().True(diffFound, "injector stats should have changed after injection")
}

// queryInjectorStats queries the system-probe for injector stats using NamedPipeCmd.exe
func (s *testInjectorStats) queryInjectorStats(forceRefresh bool) map[string]interface{} {

	if forceRefresh {
		// Query system-probe's /telemetry endpoint to trigger the telemetry scheduler to collect stats.
		_, _ = s.querySystemProbe("/telemetry")

		// The scheduler has a delay start before running down the Collect callbacks.
		// Give enough time for the delay start and collection.
		time.Sleep(10 * time.Second)
	}

	// Query system-probe's /debug/stats endpoint.
	// We cannot use the /telemetry endpoint because it returns a compressed output.
	output, err := s.querySystemProbe("/debug/stats")
	if output != "" {
		s.T().Logf("system-probe output:\n\n%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to query system-probe: %s", output)

	// Parse JSON response
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(output), &jsonOutput)
	s.Require().NoErrorf(err, "failed to parse JSON response: %s", output)

	stats, ok := jsonOutput["injector"].(map[string]interface{})
	s.Require().True(ok, "injector stats not found in JSON response")

	return stats
}

// verifyStatsStructure verifies that the stats JSON contains expected fields
func (s *testInjectorStats) verifyStatsStructure(stats map[string]interface{}) {
	// Verify that all expected counter fields are present
	expectedFields := []string{
		"processes_added_to_injection_tracker",
		"processes_removed_from_injection_tracker",
		"processes_skipped_subsystem",
		"processes_skipped_container",
		"processes_skipped_protected",
		"processes_skipped_system",
		"processes_skipped_excluded",
		"injection_attempts",
		"injection_attempt_failures",
		"injection_max_time_us",
		"injection_successes",
		"injection_failures",
		"pe_caching_failures",
		"import_directory_restoration_failures",
		"pe_memory_allocation_failures",
		"pe_injection_context_allocated",
		"pe_injection_context_cleanedup",
	}

	for _, field := range expectedFields {
		s.Require().Contains(stats, field, "stats should contain field: %s", field)

		// Verify the field value is a number (JSON unmarshals numbers as float64)
		value, ok := stats[field].(float64)
		s.Require().True(ok, "field %s should be a number, got %T", field, stats[field])
		s.Require().GreaterOrEqual(value, float64(0), "counter %s should be non-negative", field)
	}

	// Log the stats for debugging
	s.T().Logf("Injector Stats: %+v", stats)
}

// enableInjectorTelemetry enables reporting of injector telemetry via system-probe config
// Uses the read/modify/write pattern to preserve existing config settings
func (s *testInjectorStats) enableInjectorTelemetry() {
	host := s.Env().RemoteHost
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(host)
	s.Require().NoError(err)
	configPath := filepath.Join(configRoot, "system-probe.yaml")

	// Read existing config (or create empty map if file doesn't exist)
	config, err := s.readYamlConfig(configPath)
	if err != nil {
		// If file doesn't exist, start with empty config
		config = make(map[string]interface{})
	}

	// Explicitly enable injector telemetry.
	// If /telemetry supports uncompressed output, make sure to also enable
	// RAR with remote_agent_registry.enabled = true.
	config["injector"] = map[string]interface{}{
		"enable_telemetry": true,
	}

	// Write back the modified config
	err = s.writeYamlConfig(configPath, config)
	s.Require().NoErrorf(err, "failed to write system-probe config")

	// Restart system-probe to pick up the config
	err = windowscommon.RestartService(host, "datadog-system-probe")
	s.Require().NoErrorf(err, "failed to restart system-probe")

	s.waitForServiceRunning()
}

// readYamlConfig reads and unmarshals a YAML config file from the remote host
func (s *testInjectorStats) readYamlConfig(path string) (map[string]interface{}, error) {
	host := s.Env().RemoteHost
	configBytes, err := host.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := make(map[string]interface{})
	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// writeYamlConfig marshals and writes a YAML config file to the remote host
func (s *testInjectorStats) writeYamlConfig(path string, config map[string]interface{}) error {
	host := s.Env().RemoteHost
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	_, err = host.WriteFile(path, configYaml)
	return err
}

// installCurrentAgentVersionWithAPMInject installs the current agent version with APM inject via script
func (s *testInjectorStats) installCurrentAgentVersionWithAPMInject(opts ...installerwindows.Option) {
	output, err := s.InstallScript().Run(opts...)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})

	s.waitForServiceRunning()
}

func (s *testInjectorStats) waitForServiceRunning() {
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
}

func (s *testInjectorStats) logInjectorDiagnostics() {
	host := s.Env().RemoteHost

	// Log if ddinjector is running at all.
	out, err := host.Execute(`Get-Service ddinjector`)
	if err != nil {
		s.T().Errorf("failed to query ddinjector service: %v", err)
	} else {
		s.T().Logf("ddinjector service output:\n\n%s\n", out)
	}

	// Log the current config.
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(host)
	if err != nil {
		s.T().Errorf("failed to get config root from registry: %v", err)
	} else {
		configPath := filepath.Join(configRoot, "system-probe.yaml")
		config, err := s.readYamlConfig(configPath)
		if err != nil {
			s.T().Errorf("failed to read system-probe.yaml: %v", err)
		} else {
			s.T().Logf("system-probe config:\n\n%+v\n", config)
		}
	}
}

func (s *testInjectorStats) querySystemProbe(queryPath string) (string, error) {
	// PowerShell script with inline C# to query system-probe with a named pipe.
	scriptTemplate := `
$code = @"
using System;
using System.IO;
using System.IO.Pipes;
using System.Text;

public class NamedPipeClient
{
    public static string QuerySystemProbe(string pipeName, string httpPath)
    {
        using (var pipe = new NamedPipeClientStream(".", pipeName, PipeDirection.InOut))
        {
            pipe.Connect(5000); // 5 second timeout

            // Send HTTP GET request
            string request = string.Format("GET {0} HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n", httpPath);
            byte[] requestBytes = Encoding.UTF8.GetBytes(request);
            pipe.Write(requestBytes, 0, requestBytes.Length);
            pipe.Flush();

            // Read response
            using (var reader = new StreamReader(pipe, Encoding.UTF8))
            {
                string response = reader.ReadToEnd();

                // Extract JSON body from HTTP response (after headers)
                int bodyStart = response.IndexOf("\r\n\r\n");
                if (bodyStart > 0)
                {
                    return response.Substring(bodyStart + 4);
                }

                throw new Exception("Failed to parse HTTP response");
            }
        }
    }
}
"@

Add-Type -TypeDefinition $code -Language CSharp

try {
    $result = [NamedPipeClient]::QuerySystemProbe("dd_system_probe", "%s")
    Write-Output $result
} catch {
    Write-Error "Failed to query system-probe: $_"
    exit 1
}`

	script := fmt.Sprintf(scriptTemplate, queryPath)
	host := s.Env().RemoteHost
	return host.Execute(script)
}
