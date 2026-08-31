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
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/missedbytes"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: fleet-remediation

// The map key is IssueID scoped with a hostname digest, so match by prefix.
func findMissedBytesIssue(issues map[string]*healthplatformpayload.Issue) *healthplatformpayload.Issue {
	for id, iss := range issues {
		if strings.HasPrefix(id, missedbytes.IssueID+":") {
			return iss
		}
	}
	return nil
}

// Covers what the unit tests cannot: module registration, the scheduler, BuildIssue,
// the store, the forwarder, and the payload as the intake receives it. The loss is
// seeded before the bundle starts so the assertion lands on the first tick.
func TestMissedBytesSurvivesFullPipeline(t *testing.T) {
	logsmetrics.ResetMissedBytesForTest()
	t.Cleanup(logsmetrics.ResetMissedBytesForTest)

	// Stands in for a running logs agent and two lossy rotations.
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RecordMissedBytes("nginx", "web", 4096)
	logsmetrics.RecordMissedBytes("redis", "cache", 1024)

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
			// The check is gated on logs_enabled.
			cfg.SetInTest("logs_enabled", true)
			cfg.SetInTest("health_platform.enabled", true)
			cfg.SetInTest("health_platform.persist_on_kubernetes", true)
			cfg.SetInTest("health_platform.forwarder.interval", tickInterval)
			cfg.SetInTest("run_path", t.TempDir())
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

	var received *healthplatformpayload.Issue
	require.Eventually(t, func() bool {
		payloads, err := fiClient.GetAgentHealth()
		if err != nil {
			return false
		}
		for _, p := range payloads {
			if iss := findMissedBytesIssue(p.Issues); iss != nil {
				received = iss
				return true
			}
		}
		return false
	}, waitTimeout, waitInterval, "log-data-lost-after-rotation issue never reached fakeintake")

	// Field-level content is issue_test.go's job. Assert only what the trip changes:
	// both tuples folded into one issue, breakdown arriving as objects not a string.
	assert.Equal(t, missedbytes.IssueType, received.GetIssueType())
	assert.Contains(t, received.GetTitle(), "from 2 sources")

	sources := received.GetExtra().GetFields()["sources"].GetListValue().GetValues()
	require.Len(t, sources, 2)
	largest := sources[0].GetStructValue().GetFields()
	assert.Equal(t, "nginx", largest["source"].GetStringValue())
	assert.Equal(t, float64(4096), largest["bytes"].GetNumberValue())
}
