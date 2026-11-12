// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatform

import (
	"net/http"
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
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/logsdropped"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: fleet-remediation

// The map key is IssueID scoped with a hostname digest, so match by prefix.
func findLogsDroppedIssue(issues map[string]*healthplatformpayload.Issue) *healthplatformpayload.Issue {
	for id, iss := range issues {
		if strings.HasPrefix(id, logsdropped.IssueID+":") {
			return iss
		}
	}
	return nil
}

// logLines builds n log lines from one source, the way a tailer would.
func logLines(source, service string, n, size int) []*message.MessageMetadata {
	origin := message.NewOrigin(sources.NewLogSource(source, &logsconfig.LogsConfig{Source: source, Service: service}))
	metas := make([]*message.MessageMetadata, n)
	for i := range metas {
		metas[i] = &message.MessageMetadata{Origin: origin, RawDataLen: size}
	}
	return metas
}

// Covers what the unit tests cannot: a real logs destination rejecting a payload,
// then module registration, the scheduler, BuildIssue, the store, the forwarder, and
// the payload as the intake receives it. The drop happens before the bundle starts
// so the assertion lands on the first tick.
func TestLogsDroppedSurvivesFullPipeline(t *testing.T) {
	logsmetrics.ResetDroppedLogsForTest()
	t.Cleanup(logsmetrics.ResetDroppedLogsForTest)
	logsmetrics.MarkLogsAgentRunning()

	// A logs intake that rejects the API key, and one payload sent to it. The event
	// platform message riding along carries no Origin and must not be counted.
	intake := logshttp.NewTestServer(http.StatusForbidden, configmock.New(t))
	t.Cleanup(intake.Stop)
	input, output := make(chan *message.Payload), make(chan *message.Payload)
	intake.Destination.Start(input, output, nil)
	metas := append(logLines("nginx", "web", 40, 100), logLines("redis", "cache", 2, 64)...)
	input <- &message.Payload{MessageMetas: append(metas, &message.MessageMetadata{RawDataLen: 999}), Encoded: []byte("payload")}
	<-output

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
			if iss := findLogsDroppedIssue(p.Issues); iss != nil {
				received = iss
				return true
			}
		}
		return false
	}, 5*time.Second, tickInterval, "logs-dropped-before-delivery issue never reached fakeintake")

	// Field-level content is issue_test.go's job. Assert only what the trip changes:
	// both origins folded into one issue, lists arriving as objects not strings.
	assert.Equal(t, logsdropped.IssueType, received.GetIssueType())
	assert.Equal(t, healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH, received.GetSeverity())
	assert.Contains(t, received.GetTitle(), "Dropped 42 logs (4.1 kB)")
	assert.Contains(t, received.GetTitle(), "from 2 sources")

	drops := received.GetExtra().GetFields()["drops"].GetListValue().GetValues()
	require.Len(t, drops, 2)
	largest := drops[0].GetStructValue().GetFields()
	assert.Equal(t, "nginx", largest["source"].GetStringValue())
	assert.Equal(t, "auth_rejected", largest["reason"].GetStringValue())
	assert.Equal(t, float64(40), largest["logs"].GetNumberValue())

	causes := received.GetExtra().GetFields()["causes"].GetListValue().GetValues()
	require.Len(t, causes, 1)
	cause := causes[0].GetStructValue().GetFields()
	assert.Equal(t, "primary", cause["kind"].GetStringValue())
	assert.Equal(t, "127.0.0.1", cause["destination"].GetStringValue())
}
