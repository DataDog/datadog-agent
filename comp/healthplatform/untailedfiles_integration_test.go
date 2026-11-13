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
	schemanoopfx "github.com/DataDog/datadog-agent/comp/core/schema/fx-noop"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/untailedfiles"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: fleet-remediation

// The map key is IssueID scoped with a hostname digest, so match by prefix.
func findUntailedFilesIssue(issues map[string]*healthplatformpayload.Issue) *healthplatformpayload.Issue {
	for id, iss := range issues {
		if strings.HasPrefix(id, untailedfiles.IssueID+":") {
			return iss
		}
	}
	return nil
}

// Covers what the unit tests cannot: module registration, the scheduler, BuildIssue,
// the store, the forwarder, and the payload as the intake receives it. The scan is
// seeded before the bundle starts so the assertion lands on the first tick.
func TestUntailedFilesSurvivesFullPipeline(t *testing.T) {
	logsmetrics.ResetUntailedFilesForTest()
	t.Cleanup(logsmetrics.ResetUntailedFilesForTest)

	// Stands in for a running logs agent whose last scan hit the open files limit.
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RecordUntailedFiles(500, 500, []logsmetrics.UntailedFilesSource{
		{Source: "nginx", Service: "web", Path: "/var/log/nginx/*.log", Matched: 320, Untailed: 300},
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
			cfg.SetInTest("health_platform.forwarder.dd_url", fi.URL())
			cfg.SetInTest("logs_enabled", true)
			cfg.SetInTest("health_platform.enabled", true)
			cfg.SetInTest("health_platform.persist_on_kubernetes", true)
			cfg.SetInTest("health_platform.forwarder.interval", tickInterval)
			cfg.SetInTest("run_path", t.TempDir())
			return cfg
		}),
		telemetrymock.Module(),
		schemanoopfx.Module(),
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
			if iss := findUntailedFilesIssue(p.Issues); iss != nil {
				received = iss
				return true
			}
		}
		return false
	}, 5*time.Second, tickInterval, "log-files-not-tailed issue never reached fakeintake")

	assert.Equal(t, untailedfiles.IssueType, received.GetIssueType())
	assert.Equal(t, healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_MEDIUM, received.GetSeverity())
	assert.Contains(t, received.GetTitle(), "Not tailing 300 of 800")

	sources := received.GetExtra().GetFields()["sources"].GetListValue().GetValues()
	require.Len(t, sources, 1)
	assert.Equal(t, "/var/log/nginx/*.log", sources[0].GetStructValue().GetFields()["path"].GetStringValue())
}
