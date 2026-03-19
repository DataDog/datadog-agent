// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package opamptest provides helpers for functional tests of the OpAmp integration.
// Tests in this package start the otel-agent binary as a subprocess and run an
// in-process OpAmp server to observe the agent's behaviour.
//
// Set the OTEL_AGENT environment variable to the path of the otel-agent binary
// (default: bin/otel-agent/otel-agent relative to the repo root).
package opamptest

import (
	"context"
	"crypto/sha256"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	"github.com/open-telemetry/opamp-go/server/types"
	"github.com/stretchr/testify/require"
)

// agentEnv returns the common environment variables for running the otel-agent
// without a core Datadog Agent present.
func agentEnv(runPath string) []string {
	return []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"DD_OTELCOLLECTOR_ENABLED=true",
		"DD_API_KEY=test",
		"DD_SITE=datadoghq.com",
		"DD_CMD_PORT=0",
		"DD_AGENT_IPC_PORT=-1",
		"DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL=0",
		"DD_ENABLE_METADATA_COLLECTION=false",
		"DD_HOSTNAME=test-host",
		"DD_OTELCOLLECTOR_CONVERTER_FEATURES=health_check",
		"DD_RUN_PATH=" + runPath,
	}
}

// otelAgentBin returns the path to the otel-agent binary.
func otelAgentBin(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("OTEL_AGENT"); v != "" {
		return v
	}
	// Walk up from this file to the repo root.
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(thisFile, "../../../../../..")
	return filepath.Join(root, "bin/otel-agent/otel-agent")
}

// startAgent writes config to a temp file and starts the otel-agent subprocess.
// It returns the running command; callers must call cmd.Process.Kill() to stop it.
// If runDir is empty a fresh temp directory is created; pass a pre-existing directory
// to share state (e.g. the instance-UID file) between multiple runs.
func startAgent(t *testing.T, configYAML string, runDir ...string) (*exec.Cmd, string) {
	t.Helper()
	bin := otelAgentBin(t)
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("otel-agent binary not found at %s (set OTEL_AGENT env var)", bin)
	}

	var dir string
	if len(runDir) > 0 && runDir[0] != "" {
		dir = runDir[0]
	} else {
		dir = t.TempDir()
	}
	cfgFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgFile, []byte(configYAML), 0o644))

	logFile := filepath.Join(dir, "agent.log")
	f, err := os.Create(logFile)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })

	cmd := exec.Command(bin, "--config", "file:"+cfgFile)
	cmd.Env = agentEnv(dir)
	cmd.Stdout = f
	cmd.Stderr = f
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		cmd.Process.Kill() //nolint:errcheck
		cmd.Wait()         //nolint:errcheck
	})

	return cmd, logFile
}

// waitForLog polls logFile until pattern appears or timeout elapses.
func waitForLog(t *testing.T, logFile, pattern string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(logFile)
		if contains(data, pattern) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	data, _ := os.ReadFile(logFile)
	t.Logf("agent log:\n%s", data)
	return false
}

func contains(data []byte, s string) bool {
	return len(data) > 0 && (string(data) == s || findSubstring(string(data), s))
}

func findSubstring(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s[:len(sub)] == sub || findSubstring(s[1:], sub)))
}

// testServer is a minimal in-process OpAmp server for testing.
type testServer struct {
	mu       sync.Mutex
	srv      server.OpAMPServer
	messages []*protobufs.AgentToServer
	conns    []types.Connection

	// onMessage is called (while mu is held) for each AgentToServer received.
	// It may return a non-nil ServerToAgent to send back immediately.
	onMessage func(conn types.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{}
	ts.srv = server.New(nil)
	settings := server.StartSettings{
		Settings: server.Settings{
			Callbacks: types.Callbacks{
				OnConnecting: func(_ *http.Request) types.ConnectionResponse {
					return types.ConnectionResponse{
						Accept: true,
						ConnectionCallbacks: types.ConnectionCallbacks{
							OnConnected: func(_ context.Context, conn types.Connection) {
								ts.mu.Lock()
								ts.conns = append(ts.conns, conn)
								ts.mu.Unlock()
							},
							OnMessage: func(_ context.Context, conn types.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent {
								ts.mu.Lock()
								ts.messages = append(ts.messages, msg)
								var resp *protobufs.ServerToAgent
								if ts.onMessage != nil {
									resp = ts.onMessage(conn, msg)
								}
								ts.mu.Unlock()
								if resp == nil {
									resp = &protobufs.ServerToAgent{InstanceUid: msg.InstanceUid}
								}
								return resp
							},
							OnConnectionClose: func(_ types.Connection) {},
						},
					}
				},
			},
		},
		ListenEndpoint: "0.0.0.0:4320",
		ListenPath:     "/v1/opamp",
	}
	require.NoError(t, ts.srv.Start(settings))
	t.Cleanup(func() { ts.srv.Stop(context.Background()) }) //nolint:errcheck
	return ts
}

// waitForMessage blocks until at least n messages have been received or timeout elapses.
func (ts *testServer) waitForMessage(t *testing.T, n int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		got := len(ts.messages)
		ts.mu.Unlock()
		if got >= n {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	ts.mu.Lock()
	t.Logf("server received %d messages, wanted %d", len(ts.messages), n)
	ts.mu.Unlock()
	return false
}

// pushRemoteConfig sends an AgentRemoteConfig to all connected agents.
func (ts *testServer) pushRemoteConfig(ctx context.Context, configYAML string) {
	body := []byte(configYAML)
	hash := sha256.Sum256(body)
	msg := &protobufs.ServerToAgent{
		RemoteConfig: &protobufs.AgentRemoteConfig{
			Config: &protobufs.AgentConfigMap{
				ConfigMap: map[string]*protobufs.AgentConfigFile{
					"config.yaml": {Body: body},
				},
			},
			ConfigHash: hash[:],
		},
	}
	ts.mu.Lock()
	conns := make([]types.Connection, len(ts.conns))
	copy(conns, ts.conns)
	ts.mu.Unlock()

	for _, conn := range conns {
		conn.Send(ctx, msg) //nolint:errcheck
	}
}

// waitForRemoteConfigStatus blocks until a message with a non-zero RemoteConfigStatus
// arrives, then returns the status.
func (ts *testServer) waitForRemoteConfigStatus(t *testing.T, timeout time.Duration) *protobufs.RemoteConfigStatus {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		for _, msg := range ts.messages {
			if msg.RemoteConfigStatus != nil && msg.RemoteConfigStatus.Status != protobufs.RemoteConfigStatuses_RemoteConfigStatuses_UNSET {
				st := msg.RemoteConfigStatus
				ts.mu.Unlock()
				return st
			}
		}
		ts.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// firstMessageWithDescription returns the first message that contains an AgentDescription.
func (ts *testServer) firstMessageWithDescription() *protobufs.AgentToServer {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for _, msg := range ts.messages {
		if msg.AgentDescription != nil {
			return msg
		}
	}
	return nil
}

// messageCount returns the number of messages received so far.
func (ts *testServer) messageCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.messages)
}

// configWithOpamp returns a minimal OTel collector config with the opamp extension
// using WebSocket transport on localhost:4320.
func configWithOpamp(extraYAML string) string {
	return `
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
exporters:
  debug:
    verbosity: detailed
extensions:
  opamp:
    server:
      ws:
        endpoint: ws://localhost:4320/v1/opamp
service:
  extensions: [opamp]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
` + extraYAML
}

// configWithOpampHTTP returns the same minimal config but using the HTTP transport.
func configWithOpampHTTP(extraYAML string) string {
	return `
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
exporters:
  debug:
    verbosity: detailed
extensions:
  opamp:
    server:
      http:
        endpoint: http://localhost:4320/v1/opamp
service:
  extensions: [opamp]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
` + extraYAML
}

// restartServer stops the current test server and starts a fresh one on the
// same endpoint, replacing ts.srv. All accumulated messages and connections
// are cleared so callers can wait for fresh activity from the reconnecting agent.
func (ts *testServer) restartServer(t *testing.T) {
	t.Helper()
	ts.srv.Stop(context.Background()) //nolint:errcheck

	ts.mu.Lock()
	ts.messages = nil
	ts.conns = nil
	ts.mu.Unlock()

	newSrv := server.New(nil)
	settings := server.StartSettings{
		Settings: server.Settings{
			Callbacks: types.Callbacks{
				OnConnecting: func(_ *http.Request) types.ConnectionResponse {
					return types.ConnectionResponse{
						Accept: true,
						ConnectionCallbacks: types.ConnectionCallbacks{
							OnConnected: func(_ context.Context, conn types.Connection) {
								ts.mu.Lock()
								ts.conns = append(ts.conns, conn)
								ts.mu.Unlock()
							},
							OnMessage: func(_ context.Context, conn types.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent {
								ts.mu.Lock()
								ts.messages = append(ts.messages, msg)
								var resp *protobufs.ServerToAgent
								if ts.onMessage != nil {
									resp = ts.onMessage(conn, msg)
								}
								ts.mu.Unlock()
								if resp == nil {
									resp = &protobufs.ServerToAgent{InstanceUid: msg.InstanceUid}
								}
								return resp
							},
							OnConnectionClose: func(_ types.Connection) {},
						},
					}
				},
			},
		},
		ListenEndpoint: "0.0.0.0:4320",
		ListenPath:     "/v1/opamp",
	}
	require.NoError(t, newSrv.Start(settings))
	ts.srv = newSrv
}
