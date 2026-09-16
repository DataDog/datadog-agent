// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatform

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"
	pkgconfigschema "github.com/DataDog/datadog-agent/pkg/config/schema"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: fleet-remediation

// findInvalidConfigIssue returns the invalid-config issue among a health
// report's issues, if any. The issue's map key is IssueID scoped with a
// host+path suffix (invalidconfig.IssueID + ":" + digest), so lookups must
// match by prefix rather than by the bare constant.
func findInvalidConfigIssue(issues map[string]*healthplatformpayload.Issue) *healthplatformpayload.Issue {
	for id, iss := range issues {
		if strings.HasPrefix(id, invalidconfig.IssueID+":") {
			return iss
		}
	}
	return nil
}

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
// violations reach the intake without the raw invalid value.
func TestInvalidConfigExtraErrorsSurviveFullPipeline(t *testing.T) {
	requireSchema(t)
	const rawInvalidLogsEnabled = "RAW_LOGS_ENABLED_MUST_NOT_APPEAR_83d4d1"

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

	fxutil.Test[fxutil.NoDependencies](t,
		Bundle(),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component {
			cfg := config.NewMock(t)
			cfg.SetInTest("api_key", "test-api-key")
			cfg.SetInTest("dd_url", fi.URL())
			cfg.SetInTest("health_platform.enabled", true)
			cfg.SetInTest("health_platform.invalidconfig_check.enabled", true)
			cfg.SetInTest("health_platform.persist_on_kubernetes", true)
			cfg.SetInTest("health_platform.forwarder.interval", tickInterval)
			cfg.SetInTest("run_path", t.TempDir())
			cfg.SetInTest("agent_ipc.port", "not-a-number")
			cfg.SetInTest("logs_enabled", rawInvalidLogsEnabled)
			return cfg
		}),
		telemetrymock.Module(),
		hostnameinterface.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)

	const (
		waitTimeout  = 5 * time.Second
		waitInterval = 50 * time.Millisecond
	)

	var receivedIssue *healthplatformpayload.Issue
	receivedViolationIndex := -1
	require.Eventually(t, func() bool {
		payloads, err := fiClient.GetAgentHealth()
		if err != nil || len(payloads) == 0 {
			return false
		}
		for _, p := range payloads {
			if iss := findInvalidConfigIssue(p.Issues); iss != nil {
				fields := iss.GetExtra().GetFields()
				errorsStruct := fields["errors"].GetStructValue()
				if errorsStruct == nil || len(errorsStruct.GetFields()) == 0 || fields["violations_version"].GetNumberValue() != 1 {
					continue
				}
				for i, value := range fields["violations"].GetListValue().GetValues() {
					violation := value.GetStructValue().GetFields()
					expectedTypes := violation["expected_types"].GetListValue().GetValues()
					defaultValue, hasDefault := violation["default_value"]
					if violation["path"].GetStringValue() == "/logs_enabled" &&
						violation["actual_type"].GetStringValue() == "string" &&
						len(expectedTypes) == 1 && expectedTypes[0].GetStringValue() == "boolean" &&
						violation["default_status"].GetStringValue() == "known" && hasDefault &&
						defaultValue.AsInterface() == false {
						receivedIssue = iss
						receivedViolationIndex = i
						return true
					}
				}
			}
		}
		return false
	}, waitTimeout, waitInterval, "invalid-config issue with legacy and structured violations never reached fakeintake")
	require.NotNil(t, receivedIssue)

	errorsStruct := receivedIssue.GetExtra().GetFields()["errors"].GetStructValue()
	require.NotNil(t, errorsStruct, "extra.errors must reach fakeintake as a path-keyed struct")
	portErrors := errorsStruct.GetFields()["/agent_ipc/port"]
	require.NotNil(t, portErrors, "/agent_ipc/port must be present in extra.errors")
	vals := portErrors.GetListValue().GetValues()
	require.NotEmpty(t, vals)
	assert.Contains(t, vals[0].GetStringValue(), "want integer")

	fields := receivedIssue.GetExtra().GetFields()
	assert.Equal(t, float64(1), fields["violations_version"].GetNumberValue())
	violations := fields["violations"].GetListValue().GetValues()
	require.NotEqual(t, -1, receivedViolationIndex, "/logs_enabled must be present in extra.violations")
	logsViolation := violations[receivedViolationIndex].GetStructValue().GetFields()
	assert.Equal(t, "/logs_enabled", logsViolation["path"].GetStringValue())
	assert.Equal(t, "string", logsViolation["actual_type"].GetStringValue())
	expectedTypes := logsViolation["expected_types"].GetListValue().GetValues()
	require.Len(t, expectedTypes, 1)
	assert.Equal(t, "boolean", expectedTypes[0].GetStringValue())
	assert.Equal(t, "known", logsViolation["default_status"].GetStringValue())
	require.Contains(t, logsViolation, "default_value")
	assert.Equal(t, false, logsViolation["default_value"].AsInterface())

	receivedJSON, err := json.Marshal(receivedIssue)
	require.NoError(t, err)
	assert.NotContains(t, string(receivedJSON), rawInvalidLogsEnabled)
}
