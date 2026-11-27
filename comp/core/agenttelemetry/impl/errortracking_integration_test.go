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
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// The tests below run the whole errortracking path in one process, in a
// testing/synctest bubble: the component built the way the Agent builds it, the
// remote flag delivered through the subscriber it provides, an error logged
// through the Agent logger, and the payload received by an HTTP server standing
// for the intake. The sender and the intake talk over an in-memory connection,
// so the tests know when the flush ran instead of waiting for it.

// integrationFlushInterval is the flush period of the errortracking job.
const integrationFlushInterval = time.Second

// intakeRequest is what the intake received.
type intakeRequest struct {
	requestType string
	body        []byte
}

// recordingIntake records every request it receives.
type recordingIntake struct {
	mu       sync.Mutex
	requests []intakeRequest
}

func (i *recordingIntake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err == nil && r.Header.Get("Content-Encoding") == "zstd" {
		body, err = zstd.Decompress(nil, body)
	}
	if err == nil {
		i.mu.Lock()
		i.requests = append(i.requests, intakeRequest{requestType: r.Header.Get("DD-Telemetry-request-type"), body: body})
		i.mu.Unlock()
	}
	w.WriteHeader(http.StatusAccepted)
}

// agentLogsPayloads returns the agent-logs payloads received so far.
func (i *recordingIntake) agentLogsPayloads(t *testing.T) []agentLogsPayload {
	t.Helper()
	i.mu.Lock()
	defer i.mu.Unlock()
	var payloads []agentLogsPayload
	for _, req := range i.requests {
		if req.requestType != logsPayloadType {
			continue
		}
		var payload agentLogsPayload
		require.NoError(t, json.Unmarshal(req.body, &payload), "payload: %s", req.body)
		payloads = append(payloads, payload)
	}
	return payloads
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

// integrationConfig points the component at the intake. localErrortracking is
// added under agent_telemetry.errortracking and may be empty to leave the key
// unset.
func integrationConfig(localErrortracking string) string {
	return "api_key: integration-key\n" +
		"agent_telemetry:\n" +
		"  enabled: true\n" +
		"  use_compression: false\n" +
		"  dd_url: http://" + pipeListenerAddress + "\n" +
		"  errortracking:\n" +
		"    flush_interval_seconds: 1\n" +
		"    startup_jitter_seconds: 0\n" +
		localErrortracking
}

// runIntegrationScenario builds and starts the component over the given local
// configuration inside a synctest bubble and hands the scenario the handler of
// the errortracking flag, as the Remote Flags client holds it, and the intake.
func runIntegrationScenario(t *testing.T, localErrortracking string, scenario func(t *testing.T, flag pkgremoteflags.FlagHandler, intake *recordingIntake)) {
	t.Helper()
	cleanupErrortrackingSlots(t)

	// The process-wide state the components create lazily is created outside
	// the bubble: created inside, it would be bound to it.
	cfg := configmock.NewFromYAML(t, integrationConfig(localErrortracking))
	logComp := makeLogMock(t)
	telComp := makeTelMock(t)
	// Log to the console, not to a file: the file stays open until the process
	// ends, which on Windows makes the temporary directory impossible to remove.
	require.NoError(t, pkglogsetup.SetupLogger("TEST", "info", "", "", false, true, false, cfg))
	t.Cleanup(pkglog.Flush)

	synctest.Test(t, func(t *testing.T) {
		listener := newPipeListener()
		intake := &recordingIntake{}
		server := &http.Server{Handler: intake, ReadHeaderTimeout: time.Minute}
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(func() { _ = server.Close() })

		// the sender reaches the intake, and only it, over the in-memory connection
		transport := listener.transport()
		t.Cleanup(transport.CloseIdleConnections)
		previous := newTransport
		newTransport = func(config.Reader) http.RoundTripper { return transport }
		t.Cleanup(func() { newTransport = previous })

		lc := compdef.NewTestLifecycle(t)
		provides := NewComponent(Requires{Config: cfg, Log: logComp, Telemetry: telComp, Lc: lc})
		require.NoError(t, lc.Start(context.Background()))
		t.Cleanup(func() { _ = lc.Stop(context.Background()) })

		var flag pkgremoteflags.FlagHandler
		for _, h := range provides.Subscriber.Handlers() {
			if h.FlagName() == flagErrortracking {
				flag = h
			}
		}
		require.NotNil(t, flag, "the errortracking flag is not subscribed")

		scenario(t, flag, intake)
	})
}

// letFlushesRun gives the flush job several periods of bubble time and waits
// until everything it started is idle again.
func letFlushesRun() {
	time.Sleep(3 * integrationFlushInterval)
	synctest.Wait()
}

// Each helper logs from its own call site, because the bouncer reports a stack
// once per window: a second record from the same call site would be counted,
// not sent.
func logIntegrationErrorBeforeFlag()   { _ = pkglog.Errorf("integration: before the flag") }
func logIntegrationErrorFromFlagOn()   { _ = pkglog.Errorf("integration: flag on") }
func logIntegrationErrorFromLocalOff() { _ = pkglog.Errorf("integration: local off") }
func logIntegrationErrorBeforeRemoval() {
	_ = pkglog.Errorf("integration: before removal")
}
func logIntegrationErrorAfterRemoval() { _ = pkglog.Errorf("integration: after removal") }

// With the key unset, the flag turns the forwarding on: an error logged
// through the Agent logger reaches the intake as an agent-logs payload that
// carries the Go stack of the call site.
func TestErrortrackingIntegration_FlagOnForwardsErrors(t *testing.T) {
	runIntegrationScenario(t, "", func(t *testing.T, flag pkgremoteflags.FlagHandler, intake *recordingIntake) {
		// Nothing is forwarded before the flag arrives.
		logIntegrationErrorBeforeFlag()
		letFlushesRun()
		require.Empty(t, intake.agentLogsPayloads(t))

		require.NoError(t, flag.OnChange(true))
		logIntegrationErrorFromFlagOn()
		letFlushesRun()

		payloads := intake.agentLogsPayloads(t)
		require.Len(t, payloads, 1)
		assert.Equal(t, logsPayloadType, payloads[0].RequestType)
		require.Len(t, payloads[0].Payload.Logs, 1)
		entry := payloads[0].Payload.Logs[0]
		assert.Equal(t, "ERROR", entry.Level)
		assert.Empty(t, entry.Message, "the message is user data and must not be sent")
		assert.GreaterOrEqual(t, entry.Count, 1)
		assert.Contains(t, entry.Tags, "git.repository_url:https://github.com/DataDog/datadog-agent")
		assert.Contains(t, entry.Tags, "agent.flavor:agent")

		// Go stack format: a function line followed by a tab-indented file:line +0xaddr line.
		assert.Contains(t, entry.StackTrace, "logIntegrationErrorFromFlagOn")
		assert.Regexp(t, `(?m)^\t\S+\.go:\d+ \+0x[0-9a-f]+$`, entry.StackTrace)
	})
}

// An explicit local false wins over the flag: the flag is accepted and nothing
// is forwarded.
func TestErrortrackingIntegration_LocalFalseWins(t *testing.T) {
	runIntegrationScenario(t, "    enabled: false\n", func(t *testing.T, flag pkgremoteflags.FlagHandler, intake *recordingIntake) {
		require.NoError(t, flag.OnChange(true))
		logIntegrationErrorFromLocalOff()
		letFlushesRun()

		assert.Empty(t, intake.agentLogsPayloads(t))
	})
}

// When the flag is removed, forwarding stops: what was logged before arrives,
// what is logged afterwards does not.
func TestErrortrackingIntegration_FlagRemovalStopsForwarding(t *testing.T) {
	runIntegrationScenario(t, "", func(t *testing.T, flag pkgremoteflags.FlagHandler, intake *recordingIntake) {
		require.NoError(t, flag.OnChange(true))
		logIntegrationErrorBeforeRemoval()
		letFlushesRun()
		payloads := intake.agentLogsPayloads(t)
		require.Len(t, payloads, 1)
		require.Len(t, payloads[0].Payload.Logs, 1)
		assert.True(t, strings.Contains(payloads[0].Payload.Logs[0].StackTrace, "logIntegrationErrorBeforeRemoval"))

		flag.OnNoConfig()
		logIntegrationErrorAfterRemoval()
		letFlushesRun()

		assert.Len(t, intake.agentLogsPayloads(t), 1, "nothing may arrive after the flag was removed")
	})
}
