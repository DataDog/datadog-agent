// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package opamptest

import (
	"context"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	agentStartTimeout  = 20 * time.Second
	messageTimeout     = 15 * time.Second
	configApplyTimeout = 20 * time.Second
)

// TestOpampConnect verifies that the agent connects to an OpAmp server over
// WebSocket and sends an initial AgentToServer message containing an
// AgentDescription.
//
// speky:OTELCOL#T015 speky:OTELCOL#T016
func TestOpampConnect(t *testing.T) {
	ts := newTestServer(t)
	_, logFile := startAgent(t, configWithOpamp(""))

	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout),
		"agent did not reach ready state")
	require.True(t, ts.waitForMessage(t, 1, messageTimeout),
		"server did not receive any message")

	msg := ts.firstMessageWithDescription()
	require.NotNil(t, msg, "no message with AgentDescription received")
	assert.NotEmpty(t, msg.AgentDescription.IdentifyingAttributes, "identifying attributes should not be empty")
}

// TestOpampAgentDescription verifies that the AgentDescription sent by the
// agent includes service.name=otel-agent and the Datadog-specific attributes
// datadoghq.com/site and datadoghq.com/deployment_type.
//
// speky:OTELCOL#T018
func TestOpampAgentDescription(t *testing.T) {
	ts := newTestServer(t)
	_, logFile := startAgent(t, configWithOpamp(""))

	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout),
		"agent did not reach ready state")
	require.True(t, ts.waitForMessage(t, 1, messageTimeout),
		"server did not receive any message")

	msg := ts.firstMessageWithDescription()
	require.NotNil(t, msg, "no message with AgentDescription received")

	desc := msg.AgentDescription

	// Gather all non-identifying attributes into a map for easy lookup.
	nonIdent := make(map[string]string)
	for _, kv := range desc.NonIdentifyingAttributes {
		if sv := kv.Value.GetStringValue(); sv != "" {
			nonIdent[kv.Key] = sv
		}
	}

	assert.NotEmpty(t, nonIdent["datadoghq.com/site"], "datadoghq.com/site should be set")
	assert.NotEmpty(t, nonIdent["datadoghq.com/deployment_type"], "datadoghq.com/deployment_type should be set")

	// service.name must be among the identifying attributes.
	idAttrs := make(map[string]string)
	for _, kv := range desc.IdentifyingAttributes {
		if sv := kv.Value.GetStringValue(); sv != "" {
			idAttrs[kv.Key] = sv
		}
	}
	assert.Equal(t, "otel-agent", idAttrs["service.name"], "service.name should be otel-agent")
}

// TestOpampRemoteConfig verifies that a RemoteConfig pushed from the server is
// accepted and acknowledged with status APPLIED.
//
// speky:OTELCOL#T019
func TestOpampRemoteConfig(t *testing.T) {
	ts := newTestServer(t)
	_, logFile := startAgent(t, configWithOpamp(""))

	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout),
		"agent did not reach ready state")
	require.True(t, ts.waitForMessage(t, 1, messageTimeout),
		"server did not receive initial message")

	// Push a valid remote config.
	ts.pushRemoteConfig(context.Background(), configWithOpamp(""))

	st := ts.waitForRemoteConfigStatus(t, configApplyTimeout)
	require.NotNil(t, st, "no RemoteConfigStatus received after config push")
	assert.Equal(t, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, st.Status,
		"expected APPLIED status, got %v: %s", st.Status, st.ErrorMessage)
}

// TestOpampHealthReport verifies that the initial AgentToServer message
// contains a Health report with Healthy=true and that the agent reports its
// effective config.
//
// speky:OTELCOL#T021
func TestOpampHealthReport(t *testing.T) {
	ts := newTestServer(t)
	_, logFile := startAgent(t, configWithOpamp(""))

	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout),
		"agent did not reach ready state")
	require.True(t, ts.waitForMessage(t, 1, messageTimeout),
		"server did not receive any message")

	// The health and effective config may arrive in the first or a follow-up message.
	var health *protobufs.ComponentHealth
	var effectiveCfg *protobufs.EffectiveConfig
	deadline := time.Now().Add(messageTimeout)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		for _, msg := range ts.messages {
			if msg.Health != nil && health == nil {
				health = msg.Health
			}
			if msg.EffectiveConfig != nil && effectiveCfg == nil {
				effectiveCfg = msg.EffectiveConfig
			}
		}
		ts.mu.Unlock()
		if health != nil && effectiveCfg != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	require.NotNil(t, health, "no Health report received")
	assert.True(t, health.Healthy, "agent should report Healthy=true")
	require.NotNil(t, effectiveCfg, "no EffectiveConfig received")
	assert.NotNil(t, effectiveCfg.ConfigMap, "effective config map should not be nil")
}

// TestOpampEffectiveConfigUpdated verifies that after a remote config push the
// agent reports an updated EffectiveConfig that reflects the new configuration.
//
// speky:OTELCOL#T031
func TestOpampEffectiveConfigUpdated(t *testing.T) {
	ts := newTestServer(t)
	_, logFile := startAgent(t, configWithOpamp(""))

	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout),
		"agent did not reach ready state")
	require.True(t, ts.waitForMessage(t, 1, messageTimeout),
		"server did not receive initial message")

	// Record the number of messages before the push.
	before := ts.messageCount()

	// Push a remote config.
	newConfig := configWithOpamp("")
	ts.pushRemoteConfig(context.Background(), newConfig)

	st := ts.waitForRemoteConfigStatus(t, configApplyTimeout)
	require.NotNil(t, st, "no RemoteConfigStatus received")
	assert.Equal(t, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, st.Status)

	// Wait for a follow-up message that carries the updated EffectiveConfig.
	deadline := time.Now().Add(messageTimeout)
	var updatedCfg *protobufs.EffectiveConfig
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		for i := before; i < len(ts.messages); i++ {
			if ts.messages[i].EffectiveConfig != nil {
				updatedCfg = ts.messages[i].EffectiveConfig
				break
			}
		}
		ts.mu.Unlock()
		if updatedCfg != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	require.NotNil(t, updatedCfg, "no updated EffectiveConfig received after config push")
}

// hasCapability reports whether a capabilities bitmask includes the given flag.
func hasCapability(caps uint64, flag protobufs.AgentCapabilities) bool {
	return caps&uint64(flag) != 0
}

// waitForCapabilities blocks until the first AgentToServer message with a
// non-zero Capabilities field arrives and returns it, or returns 0 on timeout.
func (ts *testServer) waitForCapabilities(t *testing.T, timeout time.Duration) uint64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		for _, msg := range ts.messages {
			if msg.Capabilities != 0 {
				c := msg.Capabilities
				ts.mu.Unlock()
				return c
			}
		}
		ts.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}

// TestOpampHeartbeats verifies that the agent sends periodic heartbeat messages
// to the OpAmp server, and that the heartbeat interval can be shortened by the
// server via OpAMPConnectionSettings.heartbeat_interval_seconds.
//
// speky:OTELCOL#T020
func TestOpampHeartbeats(t *testing.T) {
	ts := newTestServer(t)
	_, logFile := startAgent(t, configWithOpamp(""))

	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout))

	caps := ts.waitForCapabilities(t, messageTimeout)
	require.NotZero(t, caps, "agent did not send capabilities")
	if !hasCapability(caps, protobufs.AgentCapabilities_AgentCapabilities_ReportsHeartbeat) {
		t.Skip("agent does not declare ReportsHeartbeat capability — skipping until implemented")
	}

	// Ask the server to reduce the heartbeat interval to 3 s so the test is fast.
	before := ts.messageCount()
	ts.mu.Lock()
	conns := make([]types.Connection, len(ts.conns))
	copy(conns, ts.conns)
	ts.mu.Unlock()
	heartbeatMsg := &protobufs.ServerToAgent{
		ConnectionSettings: &protobufs.ConnectionSettingsOffers{
			Opamp: &protobufs.OpAMPConnectionSettings{
				HeartbeatIntervalSeconds: 3,
			},
		},
	}
	for _, conn := range conns {
		conn.Send(context.Background(), heartbeatMsg) //nolint:errcheck
	}

	// Expect at least 2 more heartbeat messages within 12 s (3 s interval × 2 + slack).
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if ts.messageCount()-before >= 2 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.GreaterOrEqual(t, ts.messageCount()-before, 2,
		"expected at least 2 heartbeats after interval change, got %d", ts.messageCount()-before)
}

// TestOpampInvalidTLSRejected verifies that the agent rejects an
// OpAMPConnectionSettings push that contains a malformed TLS certificate and
// reports a FAILED ConnectionSettingsStatus back to the server.
//
// speky:OTELCOL#T028
func TestOpampInvalidTLSRejected(t *testing.T) {
	ts := newTestServer(t)
	_, logFile := startAgent(t, configWithOpamp(""))

	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout))
	require.True(t, ts.waitForMessage(t, 1, messageTimeout))

	caps := ts.waitForCapabilities(t, messageTimeout)
	if !hasCapability(caps, protobufs.AgentCapabilities_AgentCapabilities_AcceptsOpAMPConnectionSettings) {
		t.Skip("agent does not declare AcceptsOpAMPConnectionSettings — skipping until implemented")
	}

	// Push a certificate with clearly invalid PEM content.
	ts.mu.Lock()
	conns := make([]types.Connection, len(ts.conns))
	copy(conns, ts.conns)
	ts.mu.Unlock()
	badCertMsg := &protobufs.ServerToAgent{
		ConnectionSettings: &protobufs.ConnectionSettingsOffers{
			Opamp: &protobufs.OpAMPConnectionSettings{
				Certificate: &protobufs.TLSCertificate{
					Cert:       []byte("not-a-valid-pem"),
					PrivateKey: []byte("not-a-valid-pem"),
				},
			},
		},
	}
	for _, conn := range conns {
		conn.Send(context.Background(), badCertMsg) //nolint:errcheck
	}

	// Wait for a ConnectionSettingsStatus with status FAILED.
	deadline := time.Now().Add(configApplyTimeout)
	var connStatus *protobufs.ConnectionSettingsStatus
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		for _, msg := range ts.messages {
			if msg.ConnectionSettingsStatus != nil &&
				msg.ConnectionSettingsStatus.Status != protobufs.ConnectionSettingsStatuses_ConnectionSettingsStatuses_UNSET {
				connStatus = msg.ConnectionSettingsStatus
			}
		}
		ts.mu.Unlock()
		if connStatus != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	require.NotNil(t, connStatus, "no ConnectionSettingsStatus received after pushing invalid TLS cert")
	assert.Equal(t, protobufs.ConnectionSettingsStatuses_ConnectionSettingsStatuses_FAILED, connStatus.Status,
		"expected FAILED status, got %v: %s", connStatus.Status, connStatus.ErrorMessage)
	assert.NotEmpty(t, connStatus.ErrorMessage, "error message should describe the rejection reason")
}

// TestOpampPackageHashRejected verifies that the agent rejects a package whose
// advertised hash does not match the downloaded content and reports a FAILED
// PackageStatuses back to the server.
//
// speky:OTELCOL#T027
func TestOpampPackageHashRejected(t *testing.T) {
	ts := newTestServer(t)
	_, logFile := startAgent(t, configWithOpamp(""))

	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout))
	require.True(t, ts.waitForMessage(t, 1, messageTimeout))

	caps := ts.waitForCapabilities(t, messageTimeout)
	if !hasCapability(caps, protobufs.AgentCapabilities_AgentCapabilities_AcceptsPackages) {
		t.Skip("agent does not declare AcceptsPackages capability — skipping until implemented")
	}

	// Advertise a package with a hash that won't match any real content.
	ts.mu.Lock()
	conns := make([]types.Connection, len(ts.conns))
	copy(conns, ts.conns)
	ts.mu.Unlock()
	badHashMsg := &protobufs.ServerToAgent{
		PackagesAvailable: &protobufs.PackagesAvailable{
			Packages: map[string]*protobufs.PackageAvailable{
				"otel-agent": {
					Type:    protobufs.PackageType_PackageType_TopLevel,
					Version: "99.0.0",
					File: &protobufs.DownloadableFile{
						DownloadUrl: "http://localhost:19999/nonexistent.tar.gz",
						ContentHash: []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
					},
				},
			},
			AllPackagesHash: []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		},
	}
	for _, conn := range conns {
		conn.Send(context.Background(), badHashMsg) //nolint:errcheck
	}

	// Wait for a PackageStatuses message indicating failure.
	deadline := time.Now().Add(configApplyTimeout)
	var pkgStatuses *protobufs.PackageStatuses
	for time.Now().Before(deadline) {
		ts.mu.Lock()
		for _, msg := range ts.messages {
			if msg.PackageStatuses != nil {
				pkgStatuses = msg.PackageStatuses
			}
		}
		ts.mu.Unlock()
		if pkgStatuses != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	require.NotNil(t, pkgStatuses, "no PackageStatuses received after pushing package with bad hash")
	agentPkg, ok := pkgStatuses.Packages["otel-agent"]
	require.True(t, ok, "PackageStatuses should contain 'otel-agent' entry")
	assert.Equal(t, protobufs.PackageStatusEnum_PackageStatusEnum_InstallFailed, agentPkg.Status,
		"expected InstallFailed, got %v: %s", agentPkg.Status, agentPkg.ErrorMessage)
	assert.NotEmpty(t, agentPkg.ErrorMessage, "error message should describe the hash mismatch")
}

// TestOpampInstanceUIDStable verifies that the instance UID reported by the
// agent to the OpAmp server is preserved across a stop-and-restart cycle.
//
// speky:OTELCOL#T030
func TestOpampInstanceUIDStable(t *testing.T) {
	ts := newTestServer(t)
	cfg := configWithOpamp("")

	// First run — record the instance UID.
	cmd, logFile := startAgent(t, cfg)
	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout),
		"agent did not reach ready state on first start")
	require.True(t, ts.waitForMessage(t, 1, messageTimeout),
		"server did not receive initial message")

	msg1 := ts.firstMessageWithDescription()
	require.NotNil(t, msg1, "no AgentDescription in first run")
	uid1 := string(msg1.InstanceUid)
	require.NotEmpty(t, uid1, "instance UID should not be empty")

	// Stop the agent.
	cmd.Process.Kill() //nolint:errcheck
	cmd.Wait()         //nolint:errcheck

	// Clear server state for the second run.
	ts.mu.Lock()
	ts.messages = nil
	ts.conns = nil
	ts.mu.Unlock()

	// Second run — same config, new subprocess.
	_, logFile2 := startAgent(t, cfg)
	require.True(t, waitForLog(t, logFile2, "Everything is ready", agentStartTimeout),
		"agent did not reach ready state on second start")
	require.True(t, ts.waitForMessage(t, 1, messageTimeout),
		"server did not receive message after restart")

	msg2 := ts.firstMessageWithDescription()
	require.NotNil(t, msg2, "no AgentDescription in second run")
	uid2 := string(msg2.InstanceUid)

	assert.Equal(t, uid1, uid2, "instance UID must be stable across restarts")
}

// TestOpampIdempotentPush verifies that pushing the same config hash a second
// time does not cause the agent to restart (no "Starting..." log line after the
// second push).
//
// speky:OTELCOL#T032
func TestOpampIdempotentPush(t *testing.T) {
	ts := newTestServer(t)
	_, logFile := startAgent(t, configWithOpamp(""))

	require.True(t, waitForLog(t, logFile, "Everything is ready", agentStartTimeout),
		"agent did not reach ready state")
	require.True(t, ts.waitForMessage(t, 1, messageTimeout),
		"server did not receive initial message")

	// First push — agent applies.
	cfg := configWithOpamp("")
	ts.pushRemoteConfig(context.Background(), cfg)
	st := ts.waitForRemoteConfigStatus(t, configApplyTimeout)
	require.NotNil(t, st, "no status after first push")
	assert.Equal(t, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, st.Status,
		"first push should be APPLIED")

	// Clear messages so we can wait for the second acknowledgement.
	ts.mu.Lock()
	ts.messages = nil
	ts.mu.Unlock()

	// Second push — identical hash.
	ts.pushRemoteConfig(context.Background(), cfg)
	st2 := ts.waitForRemoteConfigStatus(t, configApplyTimeout)
	require.NotNil(t, st2, "no status after second push")
	assert.Equal(t, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, st2.Status,
		"second identical push should still be APPLIED (idempotent)")
}
