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

// findMissedBytesIssue returns the log-data-lost-after-rotation issue among a
// health report's issues, if any. The map key is IssueID scoped with a hostname
// digest, so lookups match by prefix.
func findMissedBytesIssue(issues map[string]*healthplatformpayload.Issue) *healthplatformpayload.Issue {
	for id, iss := range issues {
		if strings.HasPrefix(id, missedbytes.IssueID+":") {
			return iss
		}
	}
	return nil
}

// TestMissedBytesSurvivesFullPipeline exercises what the missedbytes unit tests
// cannot: module registration, the scheduler running the check, BuildIssue, the
// store, the forwarder, and the payload as the intake receives it.
//
// The loss is seeded before the bundle starts so the assertion lands on the
// scheduler's first tick rather than waiting out its 15-minute interval.
func TestMissedBytesSurvivesFullPipeline(t *testing.T) {
	logsmetrics.ResetMissedBytesForTest()
	t.Cleanup(logsmetrics.ResetMissedBytesForTest)

	// Stands in for the file launcher and two lossy rotations.
	logsmetrics.MarkFileTailingActive()
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

	assert.Equal(t, missedbytes.IssueName, received.GetIssueName())
	assert.Equal(t, missedbytes.IssueType, received.GetIssueType())
	assert.Equal(t, "logs_pipeline", received.GetCategory())
	assert.Equal(t, "logs", received.GetSource())
	assert.Equal(t, "logs-agent", received.GetLocation())
	assert.Equal(t, healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH, received.GetSeverity())
	assert.Contains(t, received.GetTags(), "rotation")

	// One issue for the whole host, so both tuples have to be inside it.
	assert.Contains(t, received.GetTitle(), "from 2 sources")
	assert.Contains(t, received.GetDescription(), "nginx/web")
	assert.Contains(t, received.GetDescription(), "redis/cache")

	fields := received.GetExtra().GetFields()
	assert.Equal(t, float64(5120), fields["bytes_missed_24h"].GetNumberValue())
	assert.Equal(t, float64(2), fields["rotation_count_24h"].GetNumberValue())
	assert.Equal(t, float64(2), fields["source_count"].GetNumberValue())
	assert.Equal(t, float64(0), fields["sources_omitted"].GetNumberValue())

	// The breakdown has to arrive as a list of objects rather than an opaque
	// JSON string, ordered largest loss first.
	sources := fields["sources"].GetListValue().GetValues()
	require.Len(t, sources, 2)
	largest := sources[0].GetStructValue().GetFields()
	assert.Equal(t, "nginx", largest["source"].GetStringValue())
	assert.Equal(t, "web", largest["service"].GetStringValue())
	assert.Equal(t, float64(4096), largest["bytes"].GetNumberValue())
	assert.Equal(t, float64(1), largest["rotations"].GetNumberValue())
}
