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
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/logsprofile"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: fleet-remediation

// The map key is IssueID scoped with a hostname digest, so match by prefix.
func findLogsProfileIssue(issues map[string]*healthplatformpayload.Issue) *healthplatformpayload.Issue {
	for id, iss := range issues {
		if strings.HasPrefix(id, logsprofile.IssueID+":") {
			return iss
		}
	}
	return nil
}

// Covers what the unit tests cannot: module registration, the scheduler, the real config plan,
// the store, the forwarder, and Extra.recommendation as the intake receives it.
func TestLogsProfileRecommendationSurvivesFullPipeline(t *testing.T) {
	logsmetrics.ResetMissedBytesForTest()
	logsmetrics.ResetPipelineMonitorForTest()
	t.Cleanup(logsmetrics.ResetMissedBytesForTest)
	t.Cleanup(logsmetrics.ResetPipelineMonitorForTest)

	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RegisterFakePipelineMonitorForTest([]logsmetrics.ComponentSnapshot{
		logsmetrics.SaturatedSnapshotForTest("processor", "0", 0.1, 0, false),
		logsmetrics.SaturatedSnapshotForTest("destination_reliable_0", "0", 0.98, 29*time.Minute, true),
	})

	// The first tick only seeds the loss baseline, so loss must keep arriving after it.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
				logsmetrics.DestinationLogsDropped.Add("destination_reliable_0", 1)
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		<-done
	})

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
			cfg.SetInTest("logs_enabled", true)
			cfg.SetInTest("health_platform.enabled", true)
			cfg.SetInTest("health_platform.persist_on_kubernetes", true)
			cfg.SetInTest("health_platform.forwarder.interval", tickInterval)
			cfg.SetInTest("health_platform.logs_profile_recommendation.interval", tickInterval)
			cfg.SetInTest("health_platform.logs_profile_recommendation.efficiency_min_saturated_30m", time.Hour)
			cfg.SetInTest("run_path", t.TempDir())
			return cfg
		}),
		telemetrymock.Module(),
		hostnameinterface.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)

	var received *healthplatformpayload.Issue
	require.Eventually(t, func() bool {
		payloads, err := fiClient.GetAgentHealth()
		if err != nil {
			return false
		}
		for _, p := range payloads {
			if iss := findLogsProfileIssue(p.Issues); iss != nil {
				received = iss
				return true
			}
		}
		return false
	}, 10*time.Second, tickInterval, "logs-performance-profile-recommended issue never reached fakeintake")

	assert.Equal(t, logsprofile.IssueType, received.GetIssueType())
	assert.Equal(t, healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_MEDIUM, received.GetSeverity())

	rec := received.GetExtra().GetFields()["recommendation"].GetStructValue().GetFields()
	require.NotEmpty(t, rec, "recommendation must arrive as an object, not a string")
	assert.Equal(t, "logs_performance_profile", rec["kind"].GetStringValue())
	assert.Equal(t, "high-concurrency", rec["profile"].GetStringValue())
	assert.Equal(t, float64(1), rec["profile_version"].GetNumberValue())
	assert.NotEmpty(t, rec["changes"].GetListValue().GetValues())

	evidence := rec["evidence"].GetListValue().GetValues()
	require.NotEmpty(t, evidence, "evidence must arrive as a list")
	for _, v := range evidence {
		assert.NotEmpty(t, v.GetStringValue(), "evidence elements must be strings")
	}
}
