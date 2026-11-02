// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !e2eunit

// This file tests the injector statistics reporting functionality through system-probe.
package installer

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	// ddInjectorV3CountersTestVersion is the first APM inject release shipping a ddinjector
	// driver (1.0.0) that exposes the V3 counters. Release packages are not published to the
	// beta registry, so it is pulled from the stable one.
	ddInjectorV3CountersTestVersion = "0.72.0-1"
)

// runDefaultSkipListProcessScript launches a copy of whoami.exe named after an entry of the
// driver's default skip list. It runs outside System32, which the driver filters before the
// skip list, so the process is attributed to the default skip list.
const runDefaultSkipListProcessScript = `
$fixtureRoot = "C:\ddinjector-e2e"
New-Item -Path $fixtureRoot -ItemType Directory -Force | Out-Null
$dst = Join-Path $fixtureRoot "docker.exe"
Copy-Item "C:\Windows\System32\whoami.exe" $dst -Force

$env:DD_INJECT_LOG_SINKS = "stdout"
$env:DD_INJECT_LOG_LEVEL = "debug"

& $dst
`

type testInjectorStats struct {
	baseAPMInjectSuite
}

// TestInjectorStats tests querying injector statistics via system-probe
func TestInjectorStats(t *testing.T) {
	e2e.Run(t, &testInjectorStats{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testInjectorStats) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.baseAPMInjectSuite.AfterTest(suiteName, testName)
}

// TestQueryStatsViaSystemProbe tests querying injector stats through system-probe HTTP endpoint
func (s *testInjectorStats) TestQueryStatsViaSystemProbe() {
	// Install agent with APM inject enabled
	s.installCurrentAgentVersionWithAPMInject(
		WithExtraEnvVars(map[string]string{
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
		WithExtraEnvVars(map[string]string{
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

	// Verify that some counters have increased (at least one injection should have occurred)
	// Note: We can't guarantee specific counters will increase in all test environments,
	// but we can verify the stats endpoint is working and returning valid data
	updatedStats := s.waitForInjectorStats(func(c *assert.CollectT, updatedStats map[string]interface{}) {
		// If last_check_timestamp stays 0, the Collect callback never ran: the diagnostics
		// logged on timeout show whether ddinjector is running and what system-probe.yaml holds.
		ts, ok := updatedStats["last_check_timestamp"].(float64)
		if !assert.True(c, ok, "last_check_timestamp should be in stats: %+v", updatedStats) ||
			!assert.Greater(c, ts, float64(0), "stats did not refresh") {
			return
		}

		diffFound := false
		for k, v1 := range initialStats {
			v2, ok := updatedStats[k]
			if !assert.True(c, ok, "updated injector stats should have key: %s", k) {
				return
			}
			// The driver version is not a counter; only counters are expected to move.
			if k == "driver_version" {
				continue
			}
			if v1.(float64) != v2.(float64) {
				diffFound = true
				break
			}
		}
		assert.True(c, diffFound, "injector stats should have changed after injection")
	}, "injector stats should refresh after injection")
	s.verifyStatsStructure(updatedStats)

	s.T().Logf("Initial stats: %+v", initialStats)
	s.T().Logf("Updated stats: %+v", updatedStats)
}

// waitForInjectorStats polls system-probe until check passes on the injector stats, and returns
// the stats it passed on.
//
// A single read after a fixed sleep can return stats from before the change under test, as two
// caches sit in front of the driver: the counters move only when a Collect runs, which the
// /telemetry query triggers at most every 5s, and system-probe rebuilds /debug/stats every 15s.
func (s *testInjectorStats) waitForInjectorStats(check func(c *assert.CollectT, stats map[string]interface{}), msgAndArgs ...interface{}) map[string]interface{} {
	var stats map[string]interface{}
	ok := s.EventuallyWithT(func(c *assert.CollectT) {
		_, _ = s.querySystemProbe("/telemetry")
		var err error
		stats, err = s.fetchInjectorStats()
		if !assert.NoError(c, err) {
			return
		}
		check(c, stats)
	}, 1*time.Minute, 5*time.Second, msgAndArgs...)
	if !ok {
		s.logInjectorDiagnostics()
		s.T().FailNow()
	}
	return stats
}

// TestDefaultSkipListCounter tests that the V3 skip-list counters track a process skipped by the
// driver's default skip list.
func (s *testInjectorStats) TestDefaultSkipListCounter() {
	flake.Mark(s.T())

	s.installCurrentAgentVersionWithAPMInject(
		WithExtraEnvVars(map[string]string{
			"DD_APM_INSTRUMENTATION_ENABLED":                      "host",
			"DD_INSTALLER_REGISTRY_URL":                           consts.StableS3OCIRegistry,
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT": ddInjectorV3CountersTestVersion,
			"DD_APM_INSTRUMENTATION_LIBRARIES":                    "dotnet:3",
		}),
	)
	s.assertSuccessfulPromoteExperiment()
	s.enableInjectorTelemetry()
	s.waitForServiceRunning()
	s.assertDriverInjections(true)
	defer func() {
		_, err := s.Env().RemoteHost.Execute(`Remove-Item -Path "C:\ddinjector-e2e" -Recurse -Force -ErrorAction SilentlyContinue`)
		s.Assert().NoError(err, "should clean up the DDInjector skip list fixture")
	}()

	before := s.queryInjectorStats(true)
	s.assertDriverVersion(before)

	output, err := s.Env().RemoteHost.Execute(runDefaultSkipListProcessScript)
	s.Require().NoError(err)
	s.Require().NotContains(output, "main executable path", "a process on the default skip list should not be injected")

	counters := []string{"processes_skipped_default", "processes_skipped_excluded"}
	for _, counter := range counters {
		_, ok := before[counter].(float64)
		s.Require().True(ok, "%s should be a number: %+v", counter, before)
	}
	s.waitForInjectorStats(func(c *assert.CollectT, after map[string]interface{}) {
		for _, counter := range counters {
			afterValue, ok := after[counter].(float64)
			if assert.True(c, ok, "%s should be a number: %+v", counter, after) {
				assert.Greater(c, afterValue, before[counter].(float64), "%s should count the skipped process", counter)
			}
		}
	}, "the skip-list counters should count the skipped process")
}

// assertDriverVersion verifies that the stats report the file version of the installed driver.
func (s *testInjectorStats) assertDriverVersion(stats map[string]interface{}) {
	// The agent reports the fixed file version (VS_FIXEDFILEINFO), not the free-form
	// FileVersion string, so compare against the same numeric parts. The driver binary is
	// located through its service, as the agent does, rather than at a fixed path. PathName is
	// in the NT object-manager form (\??\C:\...), which Get-Item does not accept.
	output, err := s.Env().RemoteHost.Execute(`$p = (Get-CimInstance Win32_SystemDriver -Filter "Name='ddinjector'").PathName -replace '^\\\?\?\\', ''; $v = (Get-Item $p).VersionInfo; "{0}.{1}.{2}.{3}" -f $v.FileMajorPart, $v.FileMinorPart, $v.FileBuildPart, $v.FilePrivatePart`)
	s.Require().NoError(err)
	expected := strings.TrimSpace(output)
	s.Require().NotEqual("0.0.0.0", expected, "the DDInjector driver should carry a file version")
	s.Assert().Equal(expected, stats["driver_version"], "driver_version should match the installed driver: %+v", stats)
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
		"processes_automatically_added_to_skip_list",
		"processes_skipped_default",
		"processes_skipped_custom",
		"processes_skipped_crash_recovery",
		"processes_skipped_signature_policy",
		"processes_signature_policy_unknown",
		"processes_skipped_short_name",
		"crashes_during_injection",
		"crashes_post_injection",
		"boot_recovery_crash_boots_detected",
		"boot_recovery_driver_self_disabled",
		"boot_recovery_stability_timer_fired",
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
