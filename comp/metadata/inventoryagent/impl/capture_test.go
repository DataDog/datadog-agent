// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package inventoryagentimpl

import (
	"encoding/json"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/require"
)

func TestCaptureInventoryAgentOmitsCredentialsAndOwnsFields(t *testing.T) {
	p := &Payload{Hostname: "native-host", UUID: "native-uuid", Timestamp: 123456789,
		Metadata: agentMetadata{"agent_version": "7.85.0", "package_version": "7.85.0-1", "flavor": "agent", "infrastructure_mode": "end_user_device", "agent_startup_time_ms": int64(1234), "feature_process_enabled": true, "feature_networks_enabled": true,
			"install_method_tool": "macos", "install_method_tool_version": "1", "install_method_installer_version": "2", "hostname_source": "os", "feature_logs_enabled": true, "feature_remote_configuration_enabled": true, "system_probe_track_tcp_6_connections": true, "system_probe_max_connections_per_message": 123,
			"config_id": "credential-sentinel", "fleet_policies_applied": []string{"credential-sentinel"}, "config_dd_url": "credential-sentinel",
			"full_configuration": "api_key: credential-sentinel", "api_key": "credential-sentinel", "remote_configuration": "credential-sentinel", "agent_uuid": "credential-sentinel"}}
	before, err := p.MarshalJSON()
	require.NoError(t, err)
	at, cadence := p.CaptureInventorySchedule()
	require.True(t, at.IsZero())
	require.Zero(t, cadence)
	p.SetCaptureInventorySchedule(time.Now(), 10*time.Minute)
	owned := p.CopyCaptureInventory()
	require.GreaterOrEqual(t, p.CaptureInventorySize(), telemetrycapture.PayloadSize(telemetrycapture.Payload{Inventory: owned}))
	require.False(t, unsafe.StringData(p.Hostname) == unsafe.StringData(owned.Hostname))
	require.False(t, unsafe.StringData(p.Metadata["agent_version"].(string)) == unsafe.StringData(owned.Agent.AgentVersion))
	require.Equal(t, int64(1234), owned.Agent.AgentStartupTimeMS)
	require.True(t, owned.Agent.FeatureProcessEnabled)
	require.True(t, owned.Agent.FeatureNetworksEnabled)
	require.Equal(t, "macos", owned.Agent.InstallMethodTool)
	require.Equal(t, "os", owned.Agent.HostnameSource)
	require.True(t, owned.Agent.FeatureLogsEnabled && owned.Agent.FeatureRemoteConfigurationEnabled && owned.Agent.SystemProbeTrackTCP6Connections)
	require.Equal(t, int64(123), owned.Agent.SystemProbeMaxConnectionsPerMessage)
	require.False(t, unsafe.StringData(p.Metadata["install_method_tool"].(string)) == unsafe.StringData(owned.Agent.InstallMethodTool))
	after, err := p.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, before, after, "capture must not change native payload or add schedule fields")
	encoded, err := json.Marshal(owned)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "credential-sentinel")
	p.Metadata["agent_version"], p.Hostname = "changed", "changed"
	require.Equal(t, "7.85.0", owned.Agent.AgentVersion)
	require.Equal(t, "native-host", owned.Hostname)
}
