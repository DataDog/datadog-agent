// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package integrationtests_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/admission"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/internal/fakeserver"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

const (
	interfaceStatsProfile = `
metadata:
  device:
    hostname: /openconfig/system/state/hostname
  interface:
    keys:
      interface: name
    name: /openconfig/interfaces/interface/state/name
    ifindex: /openconfig/interfaces/interface/state/ifindex
metrics:
  - path: /openconfig/interfaces/interface/state/counters/in-octets
    metric: snmp.ifHCInOctets
    type: monotonic_count
    tags:
      interface: name
  - path: /openconfig/interfaces/interface/state/counters/out-octets
    metric: snmp.ifHCOutOctets
    type: monotonic_count
    tags:
      interface: name
`
)

func TestGNMICheckLoadsFromConfDAndReportsThroughSender(t *testing.T) {
	admission.ResetGateForTesting()
	admission.SetPaceForTesting(10*time.Millisecond, 10)

	mockConfig := configmock.New(t)
	confdRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", confdRoot)
	mockConfig.SetInTest("network_devices.gnmi.enabled", true)

	server, host, port := startFakeServer(t)

	confPath, err := writeGNMIConfig(confdRoot, host, port)
	require.NoError(t, err)

	integrationConfig, _, err := providers.GetIntegrationConfigFromFile(gnmi.CheckName, confPath)
	require.NoError(t, err)
	require.Len(t, integrationConfig.Instances, 1)
	assert.Equal(t, "file:"+confPath, integrationConfig.Source)
	assert.Equal(t, gnmi.CheckName, integrationConfig.Name)

	core.WithTestCatalog(t)
	core.RegisterCheck(gnmi.CheckName, gnmi.Factory())

	loader, err := core.NewGoCheckLoader()
	require.NoError(t, err)

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	checkInstance, err := loader.Load(senderManager, integrationConfig, integrationConfig.Instances[0], 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		checkInstance.Cancel()
	})

	mockSender := mocksender.NewMockSenderWithSenderManager(checkInstance.ID(), senderManager)
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	require.NoError(t, checkInstance.Run())
	event := waitSubscribeEvent(t, server)
	require.Equal(t, 1, server.ConnectionCount())
	require.Equal(t, 1, server.ActiveStreamCount())

	publishDeviceTelemetry(t, server, event.StreamID)

	require.Eventually(t, func() bool {
		if err := checkInstance.Run(); err != nil {
			return false
		}
		return hasMonotonicCount(mockSender, "snmp.ifHCInOctets")
	}, 2*time.Second, 10*time.Millisecond)

	assert.Equal(t, 1, server.ConnectionCount(), "stream should stay open across collection intervals")
	assert.Equal(t, 1, server.ActiveStreamCount())
	assert.Equal(t, 1, server.SubscriptionCount())

	mockSender.AssertCalled(t, "MonotonicCount", "snmp.ifHCInOctets", float64(42), "", mock.Anything)
	mockSender.AssertCalled(t, "MonotonicCount", "snmp.ifHCOutOctets", float64(84), "", mock.Anything)
	mockSender.AssertCalled(t, "Rate", "snmp.ifHCInOctets.rate", float64(42), "", mock.Anything)
	mockSender.AssertCalled(t, "Rate", "snmp.ifHCOutOctets.rate", float64(84), "", mock.Anything)
	mockSender.AssertCalled(t, "Gauge", "datadog.gnmi.stream_state", mock.Anything, "", mock.Anything)
	mockSender.AssertCalled(t, "Gauge", "datadog.gnmi.received_samples", mock.Anything, "", mock.Anything)
	mockSender.AssertCalled(t, "Commit")

	time.Sleep(1100 * time.Millisecond)
	require.NoError(t, checkInstance.Run())

	mockSender.AssertCalled(t, "EventPlatformEvent", mock.Anything, "network-devices-metadata")

	metadataEvent := extractMetadataEvent(t, mockSender)
	require.Equal(t, "gnmi", string(metadataEvent.Integration))
	require.Len(t, metadataEvent.Devices, 1)
	assert.Equal(t, "gnmi-router-1", metadataEvent.Devices[0].Name)
	require.Len(t, metadataEvent.Interfaces, 1)
	assert.Equal(t, "eth0", metadataEvent.Interfaces[0].Name)

	require.NoError(t, checkInstance.Run())
	assert.Equal(t, 1, server.ActiveStreamCount(), "third interval should reuse the same stream")

	checkInstance.Cancel()

	require.Eventually(t, func() bool {
		return server.ActiveStreamCount() == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func writeGNMIConfig(confdRoot, host string, port int) (string, error) {
	profileDir := filepath.Join(confdRoot, "gnmi.d", "profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(profileDir, "interface-stats.yaml"), []byte(interfaceStatsProfile), 0o644); err != nil {
		return "", err
	}

	confDir := filepath.Join(confdRoot, "gnmi.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return "", err
	}

	confYAML := `init_config:

instances:
  - address: ` + host + `
    port: ` + strconv.Itoa(port) + `
    username: user
    password: test-password
    profile: interface-stats
    min_collection_interval: 1
    metadata_collection_interval: 1
`
	confPath := filepath.Join(confDir, "conf.yaml")
	if err := os.WriteFile(confPath, []byte(confYAML), 0o644); err != nil {
		return "", err
	}
	return confPath, nil
}

func startFakeServer(t *testing.T) (*fakeserver.Server, string, int) {
	t.Helper()

	server, err := fakeserver.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	server.SetExpectedCredentials("user", "test-password")

	host, port, err := hostPort(server.Addr())
	require.NoError(t, err)
	return server, host, port
}

func publishDeviceTelemetry(t *testing.T, server *fakeserver.Server, streamID int) {
	t.Helper()

	require.NoError(t, server.SendUpdate(streamID, fakeserver.InterfaceInOctetsUpdate("eth0", 42)))
	require.NoError(t, server.SendUpdate(streamID, fakeserver.InterfaceOutOctetsUpdate("eth0", 84)))
	require.NoError(t, server.SendUpdate(streamID, hostnameUpdate("gnmi-router-1")))
	require.NoError(t, server.SendUpdate(streamID, interfaceNameUpdate("eth0")))
	require.NoError(t, server.SendUpdate(streamID, interfaceIfIndexUpdate("eth0", 1)))
}

func hasMonotonicCount(mockSender *mocksender.MockSender, metric string) bool {
	for _, call := range mockSender.Calls {
		if call.Method == "MonotonicCount" && call.Arguments[0] == metric {
			return true
		}
	}
	return false
}

func extractMetadataEvent(t *testing.T, mockSender *mocksender.MockSender) devicemetadata.NetworkDevicesMetadata {
	t.Helper()

	var metadata devicemetadata.NetworkDevicesMetadata
	found := false
	for _, call := range mockSender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		eventType, ok := call.Arguments[1].(string)
		if !ok || eventType != "network-devices-metadata" {
			continue
		}
		rawEvent, ok := call.Arguments[0].([]byte)
		require.True(t, ok)

		require.NoError(t, json.Unmarshal(rawEvent, &metadata))
		found = true
	}

	if !found {
		t.Fatal("expected network-devices-metadata event")
	}
	return metadata
}

func hostnameUpdate(hostname string) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "openconfig"},
				{Name: "system"},
				{Name: "state"},
				{Name: "hostname"},
			},
		},
		Val: fakeserver.ScalarString(hostname),
	}
}

func interfaceNameUpdate(interfaceName string) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "openconfig"},
				{Name: "interfaces"},
				{Name: "interface", Key: map[string]string{"name": interfaceName}},
				{Name: "state"},
				{Name: "name"},
			},
		},
		Val: fakeserver.ScalarString(interfaceName),
	}
}

func interfaceIfIndexUpdate(interfaceName string, ifIndex int32) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "openconfig"},
				{Name: "interfaces"},
				{Name: "interface", Key: map[string]string{"name": interfaceName}},
				{Name: "state"},
				{Name: "ifindex"},
			},
		},
		Val: fakeserver.ScalarInt64(int64(ifIndex)),
	}
}

func waitSubscribeEvent(t *testing.T, server *fakeserver.Server) fakeserver.SubscribeEvent {
	t.Helper()
	select {
	case event := <-server.SubscribeRequests():
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscribe event")
		return fakeserver.SubscribeEvent{}
	}
}

func hostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
