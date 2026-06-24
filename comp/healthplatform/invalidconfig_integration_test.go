// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatform

import (
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
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	schedulerdef "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: agent-health

// TestInvalidConfigExtraErrorsSurviveFullPipeline exercises the complete
// pipeline: IssueReport → runner.BuildIssue → store → forwarder → fakeintake.
// Asserts that extra.errors reaches the intake as a path-keyed struct.
//
// The IssueReport is injected via the scheduler rather than via the startup
// schema check so the test does not depend on the embedded schema or the
// health_platform.invalidconfig_check.enabled flag.
// The schema checker's own correctness is covered by the unit tests in the
// invalidconfig package.
func TestInvalidConfigExtraErrorsSurviveFullPipeline(t *testing.T) {
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

	type appDeps struct {
		fx.In
		Scheduler schedulerdef.Component
	}

	deps := fxutil.Test[appDeps](t,
		Bundle(),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component {
			cfg := config.NewMock(t)
			cfg.SetInTest("api_key", "test-api-key")
			cfg.SetInTest("dd_url", fi.URL())
			cfg.SetInTest("health_platform.enabled", true)
			cfg.SetInTest("health_platform.persist_on_kubernetes", true)
			cfg.SetInTest("health_platform.forwarder.interval", tickInterval)
			cfg.SetInTest("run_path", t.TempDir())
			return cfg
		}),
		telemetrymock.Module(),
		hostnameinterface.MockModule(),
	)

	// Schedule a check that returns an IssueReport matching what the invalidconfig
	// checker would produce for agent_ipc.port = "not-a-number". The runner calls
	// InvalidConfigIssue{}.BuildIssue to convert the context to a proto Issue, so
	// this exercises the runner → store → forwarder path end-to-end.
	require.NoError(t, deps.Scheduler.Schedule("agent", func() ([]runnerdef.IssueReport, error) {
		return []runnerdef.IssueReport{{
			IssueID:   "invalid-config",
			IssueName: "Invalid Config",
			Source:    "agent",
			Context: map[string]string{
				"config_path": "/etc/datadog-agent/datadog.yaml",
				"error_count": "1",
				"error.0":     "at '/agent_ipc/port': got string, want integer",
			},
		}}, nil
	}, tickInterval, nil))

	const (
		waitTimeout  = 5 * time.Second
		waitInterval = 50 * time.Millisecond
	)

	require.Eventually(t, func() bool {
		payloads, err := fiClient.GetAgentHealth()
		if err != nil || len(payloads) == 0 {
			return false
		}
		for _, p := range payloads {
			if iss, ok := p.Issues["invalid-config"]; ok && iss != nil {
				errorsStruct := iss.GetExtra().GetFields()["errors"].GetStructValue()
				return errorsStruct != nil && len(errorsStruct.GetFields()) > 0
			}
		}
		return false
	}, waitTimeout, waitInterval, "invalid-config issue with path-keyed extra.errors never reached fakeintake")

	payloads, err := fiClient.GetAgentHealth()
	require.NoError(t, err)

	var receivedIssue *healthplatformpayload.Issue
	for _, p := range payloads {
		if iss, ok := p.Issues["invalid-config"]; ok && iss != nil {
			receivedIssue = iss
			break
		}
	}
	require.NotNil(t, receivedIssue)

	errorsStruct := receivedIssue.GetExtra().GetFields()["errors"].GetStructValue()
	require.NotNil(t, errorsStruct, "extra.errors must reach fakeintake as a path-keyed struct")
	portErrors := errorsStruct.GetFields()["/agent_ipc/port"]
	require.NotNil(t, portErrors, "/agent_ipc/port must be present in extra.errors")
	vals := portErrors.GetListValue().GetValues()
	require.NotEmpty(t, vals)
	assert.Contains(t, vals[0].GetStringValue(), "want integer")
}
