// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatform

import (
	"encoding/json"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidsysprobeconfig"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigschema "github.com/DataDog/datadog-agent/pkg/config/schema"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: fleet-remediation

// requireSchema skips the test when the compressed schema files haven't been
// generated yet (run `dda inv schema.compress`). CI always has them; local
// dev builds do not unless explicitly generated.
func requireSchema(t *testing.T) {
	t.Helper()
	if _, err := pkgconfigschema.GetCoreSchema(); err != nil {
		t.Skipf("embedded schema not available (%v); run `dda inv schema.compress`", err)
	}
}

// TestInvalidConfigExtraErrorsSurviveFullPipeline exercises the complete
// pipeline: schema violation in config → startup check → runner.BuildIssue →
// store → forwarder → fakeintake. Asserts that the legacy and structured
// violations from both config checks reach intake without configured values.
func TestInvalidConfigExtraErrorsSurviveFullPipeline(t *testing.T) {
	requireSchema(t)
	const rawInvalidLogsEnabled = "RAW_LOGS_ENABLED_MUST_NOT_APPEAR_83d4d1"
	const rawInvalidHealthPort = "RAW_HEALTH_PORT_MUST_NOT_APPEAR_42fa6e"

	ready := make(chan bool, 1)
	fi := fakeintakeserver.NewServer(
		fakeintakeserver.WithAddress("127.0.0.1:0"),
		fakeintakeserver.WithReadyChannel(ready),
	)
	fi.Start()
	require.True(t, <-ready, "fakeintake server did not become ready")
	t.Cleanup(func() { _ = fi.Stop() })

	fiClient := fakeintakeclient.NewClient(fi.URL())

	const tickInterval = 50 * time.Millisecond

	store := fxutil.Test[storedef.Component](t,
		Bundle(),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component {
			cfg := config.NewMockFromYAML(t, "logs_enabled: ENC[logs_enabled]\ndogstatsd_port: ENC[PRIVATE_UNRESOLVED_HANDLE]\n")
			cfg.SetInTest("api_key", "test-api-key")
			cfg.SetInTest("dd_url", fi.URL())
			cfg.SetInTest("health_platform.enabled", true)
			cfg.SetInTest("health_platform.invalidconfig_check.enabled", true)
			cfg.SetInTest("health_platform.persist_on_kubernetes", true)
			cfg.SetInTest("health_platform.forwarder.interval", tickInterval)
			cfg.SetInTest("run_path", t.TempDir())
			cfg.SetInTest("agent_ipc.port", "not-a-number")
			cfg.SetInTest("forwarder_apikey_validation_interval", []int{61})
			cfg.Set("logs_enabled", rawInvalidLogsEnabled, model.SourceSecret)
			return cfg
		}),
		telemetrymock.Module(),
		fx.Provide(func(t testing.TB) sysprobeconfig.Component {
			cfg := sysprobeconfigmock.NewMock(t)
			cfg.Set("system_probe_config.health_port", rawInvalidHealthPort, model.SourceFile)
			cfg.Set("system_probe_config.health_port", 0, model.SourceAgentRuntime)
			return cfg
		}),
		hostnameinterface.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)

	const (
		waitTimeout  = 5 * time.Second
		waitInterval = 50 * time.Millisecond
	)

	received := map[string]*healthplatformpayload.Issue{}
	require.Eventually(t, func() bool {
		payloads, err := fiClient.GetAgentHealth()
		if err != nil {
			return false
		}
		for _, payload := range payloads {
			for id, issue := range payload.Issues {
				if issue.IssueType == invalidconfig.IssueType || issue.IssueType == invalidsysprobeconfig.IssueType {
					received[id] = issue
				}
			}
		}
		return len(received) == 4
	}, waitTimeout, waitInterval, "four separate configuration issues must reach fakeintake")

	byPath := map[string]*healthplatformpayload.Issue{}
	for _, issue := range received {
		fields := issue.GetExtra().GetFields()
		assert.Equal(t, float64(1), fields["error_count"].GetNumberValue())
		errors := fields["errors"].GetStructValue().GetFields()
		require.Len(t, errors, 1)
		violations := fields["violations"].GetListValue().GetValues()
		require.Len(t, violations, 1)
		path := violations[0].GetStructValue().GetFields()["path"].GetStringValue()
		byPath[path] = issue
		require.Contains(t, errors, path)
		require.Len(t, issue.Remediation.Steps, 4)
		assert.Equal(t, issue.Description+" "+issue.Remediation.Steps[1].Text,
			errors[path].GetListValue().GetValues()[0].GetStringValue())
		encoded, err := json.Marshal(issue)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), rawInvalidLogsEnabled)
		assert.NotContains(t, string(encoded), rawInvalidHealthPort)
		assert.NotContains(t, string(encoded), "PRIVATE_UNRESOLVED_HANDLE")
	}
	for _, expected := range []struct {
		path, actualType, expectedType string
		defaultValue                   any
	}{
		{"/agent_ipc/port", "string", "integer", float64(0)},
		{"/logs_enabled", "string", "boolean", false},
		{"/forwarder_apikey_validation_interval", "array", "integer", float64(60)},
		{"/system_probe_config/health_port", "string", "integer", float64(0)},
	} {
		require.Contains(t, byPath, expected.path)
		violation := byPath[expected.path].Extra.GetFields()["violations"].GetListValue().GetValues()[0].GetStructValue().AsMap()
		assert.Equal(t, expected.actualType, violation["actual_type"])
		assert.Equal(t, []any{expected.expectedType}, violation["expected_types"])
		assert.Equal(t, "known", violation["default_status"])
		assert.Equal(t, expected.defaultValue, violation["default_value"])
	}
	assert.NotContains(t, byPath, "/dogstatsd_port")

	for _, tc := range []struct {
		issue                          *healthplatformpayload.Issue
		value, explanation, correction string
	}{
		{byPath["/logs_enabled"], rawInvalidLogsEnabled,
			"`/logs_enabled` expects true or false, but received a string.",
			"Set `/logs_enabled` to true or false. The default value for this setting is `false`."},
		{byPath["/system_probe_config/health_port"], rawInvalidHealthPort,
			"`/system_probe_config/health_port` expects a whole number, but received a string.",
			"Set `/system_probe_config/health_port` to a whole number. The default value for this setting is `0`."},
	} {
		receivedJSON, err := json.Marshal(tc.issue)
		require.NoError(t, err)
		assert.NotContains(t, string(receivedJSON), tc.value)
		assert.NotContains(t, string(receivedJSON), "PRIVATE_UNRESOLVED_HANDLE")
		assert.Contains(t, tc.issue.GetDescription(), tc.explanation)
		assert.NotContains(t, tc.issue.GetDescription(), "/dogstatsd_port")
		require.Len(t, tc.issue.GetRemediation().GetSteps(), 4)
		assert.Contains(t, tc.issue.GetRemediation().GetSteps()[1].Text, tc.correction)
		for _, verbose := range []bool{false, true} {
			found := false
			for _, result := range Diagnose(store, diagnose.Config{Verbose: verbose}) {
				if result.Category != tc.issue.Id {
					continue
				}
				found = true
				assert.Contains(t, result.Diagnosis, tc.explanation)
				assert.NotContains(t, result.Diagnosis+result.Remediation, tc.value)
				assert.NotContains(t, result.Diagnosis+result.Remediation, "/dogstatsd_port")
				assert.NotContains(t, result.Diagnosis+result.Remediation, "PRIVATE_UNRESOLVED_HANDLE")
				if verbose {
					assert.Contains(t, result.Remediation, tc.correction)
				} else {
					assert.Empty(t, result.Remediation)
				}
			}
			assert.True(t, found, "%s issue missing from diagnostics", tc.issue.IssueName)
		}
		t.Logf("received configuration issue: %s", receivedJSON)
	}
}
