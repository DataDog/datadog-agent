// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatform

import (
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
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"
	pkgconfigschema "github.com/DataDog/datadog-agent/pkg/config/schema"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: agent-health fleet-remediation

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
// generated yet (run `dda inv schema.generate`). CI always has them; local
// dev builds do not unless explicitly generated.
func requireSchema(t *testing.T) {
	t.Helper()
	if _, err := pkgconfigschema.GetCoreSchema(); err != nil {
		t.Skipf("embedded schema not available (%v); run `dda inv schema.generate`", err)
	}
}

// TestInvalidConfigExtraErrorsSurviveFullPipeline exercises the complete
// pipeline: schema violation in config → startup check → runner.BuildIssue →
// store → forwarder → fakeintake. Asserts that extra.errors reaches the intake
// as a path-keyed struct.
func TestInvalidConfigExtraErrorsSurviveFullPipeline(t *testing.T) {
	requireSchema(t)

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
			return cfg
		}),
		telemetrymock.Module(),
		hostnameinterface.MockModule(),
	)

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
			if iss := findInvalidConfigIssue(p.Issues); iss != nil {
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
		if iss := findInvalidConfigIssue(p.Issues); iss != nil {
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
