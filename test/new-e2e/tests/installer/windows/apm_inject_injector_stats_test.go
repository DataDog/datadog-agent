// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !e2eunit

// This file tests the injector statistics reporting functionality through system-probe.
package installer

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

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
