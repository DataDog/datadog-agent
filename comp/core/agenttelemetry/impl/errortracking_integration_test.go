// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package agenttelemetryimpl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// The tests below run the whole errortracking path in one process: the
// component built the way the Agent builds it, the remote flag delivered
// through the subscriber it provides, an error logged through the Agent
// logger, and the payload received by an HTTP server standing for the intake.

const (
	// integrationFlushInterval is the flush period of the integration tests.
	// The flush job runs on the real runner, so it is the shortest period the
	// configuration accepts.
	integrationFlushInterval = time.Second
	// integrationReceiveTimeout bounds the wait for a payload that must arrive.
	integrationReceiveTimeout = 15 * time.Second
	// integrationQuietPeriod is how long nothing must arrive for a payload that
	// must not be sent: it covers several flush periods.
	integrationQuietPeriod = 3 * integrationFlushInterval
)

// intakeRequest is what the intake received.
type intakeRequest struct {
	requestType string
	body        []byte
}

// startIntake starts an HTTP server that records every telemetry request.
func startIntake(t *testing.T) (*httptest.Server, <-chan intakeRequest) {
	t.Helper()
	requests := make(chan intakeRequest, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err == nil && r.Header.Get("Content-Encoding") == "zstd" {
			body, err = zstd.Decompress(nil, body)
		}
		if err == nil {
			requests <- intakeRequest{requestType: r.Header.Get("DD-Telemetry-request-type"), body: body}
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)
	return server, requests
}

// agentLogsPayload is the part of the agent-logs payload the tests read.
type agentLogsPayload struct {
	RequestType string `json:"request_type"`
	Payload     struct {
		Logs []struct {
			Level      string `json:"level"`
			StackTrace string `json:"stack_trace"`
			Tags       string `json:"tags"`
			Count      int    `json:"count"`
			Message    string `json:"message"`
		} `json:"logs"`
	} `json:"payload"`
}

// integrationConfig points the component at the intake and enables
// errortracking-capable agent telemetry. localErrortracking is added under
// agent_telemetry.errortracking and may be empty to leave the key unset.
func integrationConfig(intakeURL, localErrortracking string) string {
	return "api_key: integration-key\n" +
		"agent_telemetry:\n" +
		"  enabled: true\n" +
		"  use_compression: false\n" +
		"  dd_url: " + intakeURL + "\n" +
		"  errortracking:\n" +
		"    flush_interval_seconds: 1\n" +
		"    startup_jitter_seconds: 0\n" +
		localErrortracking
}

// startIntegrationAgent builds and starts the component over the given
// configuration, routes the Agent logger to a file in a temporary directory and
// returns the handler of the errortracking flag as the Remote Flags client
// holds it.
func startIntegrationAgent(t *testing.T, yaml string) pkgremoteflags.FlagHandler {
	t.Helper()
	cleanupErrortrackingSlots(t)

	cfg := configmock.NewFromYAML(t, yaml)
	lc := compdef.NewTestLifecycle(t)
	provides := NewComponent(Requires{
		Config:    cfg,
		Log:       makeLogMock(t),
		Telemetry: makeTelMock(t),
		Lc:        lc,
	})
	require.NoError(t, lc.Start(context.Background()))
	t.Cleanup(func() { _ = lc.Stop(context.Background()) })

	require.NoError(t, pkglogsetup.SetupLogger("TEST", "info", filepath.Join(t.TempDir(), "agent.log"), "", false, false, false, cfg))
	t.Cleanup(pkglog.Flush)

	for _, h := range provides.Subscriber.Handlers() {
		if h.FlagName() == flagErrortracking {
			return h
		}
	}
	require.FailNow(t, "the errortracking flag is not subscribed")
	return nil
}

// Each helper logs from its own call site, because the bouncer reports a stack
// once per window: a second record from the same call site would be counted,
// not sent.
func logIntegrationErrorFromFlagOn()   { _ = pkglog.Errorf("integration: flag on") }
func logIntegrationErrorFromLocalOff() { _ = pkglog.Errorf("integration: local off") }
func logIntegrationErrorBeforeRemoval() {
	_ = pkglog.Errorf("integration: before removal")
}
func logIntegrationErrorAfterRemoval() { _ = pkglog.Errorf("integration: after removal") }

// waitForAgentLogs returns the next agent-logs payload the intake received.
func waitForAgentLogs(t *testing.T, requests <-chan intakeRequest) agentLogsPayload {
	t.Helper()
	timeout := time.After(integrationReceiveTimeout)
	for {
		select {
		case req := <-requests:
			if req.requestType != logsPayloadType {
				continue
			}
			var payload agentLogsPayload
			require.NoError(t, json.Unmarshal(req.body, &payload), "payload: %s", req.body)
			return payload
		case <-timeout:
			require.FailNow(t, "no agent-logs payload reached the intake")
		}
	}
}

// requireNoAgentLogs fails if an agent-logs payload reaches the intake within
// the quiet period.
func requireNoAgentLogs(t *testing.T, requests <-chan intakeRequest) {
	t.Helper()
	timeout := time.After(integrationQuietPeriod)
	for {
		select {
		case req := <-requests:
			if req.requestType == logsPayloadType {
				require.FailNow(t, "unexpected agent-logs payload", "%s", req.body)
			}
		case <-timeout:
			return
		}
	}
}

// With the key unset, the flag turns the forwarding on: an error logged
// through the Agent logger reaches the intake as an agent-logs payload that
// carries the Go stack of the call site.
func TestErrortrackingIntegration_FlagOnForwardsErrors(t *testing.T) {
	intake, requests := startIntake(t)
	flag := startIntegrationAgent(t, integrationConfig(intake.URL, ""))

	// Nothing is forwarded before the flag arrives.
	logIntegrationErrorFromLocalOff()
	requireNoAgentLogs(t, requests)

	require.NoError(t, flag.OnChange(true))
	logIntegrationErrorFromFlagOn()

	payload := waitForAgentLogs(t, requests)
	assert.Equal(t, logsPayloadType, payload.RequestType)
	require.Len(t, payload.Payload.Logs, 1)
	entry := payload.Payload.Logs[0]
	assert.Equal(t, "ERROR", entry.Level)
	assert.Empty(t, entry.Message, "the message is user data and must not be sent")
	assert.GreaterOrEqual(t, entry.Count, 1)
	assert.Contains(t, entry.Tags, "git.repository_url:https://github.com/DataDog/datadog-agent")
	assert.Contains(t, entry.Tags, "agent.flavor:agent")

	// Go stack format: a function line followed by a tab-indented file:line +0xaddr line.
	assert.Contains(t, entry.StackTrace, "logIntegrationErrorFromFlagOn")
	assert.Regexp(t, `(?m)^\t\S+\.go:\d+ \+0x[0-9a-f]+$`, entry.StackTrace)
}

// An explicit local false wins over the flag: the flag is accepted and nothing
// is forwarded.
func TestErrortrackingIntegration_LocalFalseWins(t *testing.T) {
	intake, requests := startIntake(t)
	flag := startIntegrationAgent(t, integrationConfig(intake.URL, "    enabled: false\n"))

	require.NoError(t, flag.OnChange(true))
	logIntegrationErrorFromLocalOff()

	requireNoAgentLogs(t, requests)
}

// When the flag is removed, forwarding stops: what was logged before arrives,
// what is logged afterwards does not.
func TestErrortrackingIntegration_FlagRemovalStopsForwarding(t *testing.T) {
	intake, requests := startIntake(t)
	flag := startIntegrationAgent(t, integrationConfig(intake.URL, ""))

	require.NoError(t, flag.OnChange(true))
	logIntegrationErrorBeforeRemoval()
	payload := waitForAgentLogs(t, requests)
	require.Len(t, payload.Payload.Logs, 1)
	assert.True(t, strings.Contains(payload.Payload.Logs[0].StackTrace, "logIntegrationErrorBeforeRemoval"))

	flag.OnNoConfig()
	logIntegrationErrorAfterRemoval()

	requireNoAgentLogs(t, requests)
}
