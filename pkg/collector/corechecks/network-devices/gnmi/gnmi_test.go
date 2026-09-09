// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gnmi_test

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/admission"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/internal/fakeserver"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

const interfaceStatsProfile = `
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

func TestCheckRunReportsSNMPMetrics(t *testing.T) {
	admission.ResetGateForTesting()
	admission.SetPaceForTesting(10*time.Millisecond, 10)

	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)
	mockConfig.SetInTest("network_devices.gnmi.enabled", true)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", "profiles")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "interface-stats.yaml"), []byte(interfaceStatsProfile), 0o644))

	server, err := fakeserver.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	server.SetExpectedCredentials("user", "test-password")

	host, port, err := hostPort(server.Addr())
	require.NoError(t, err)

	rawInstance := []byte(`
address: ` + host + `
port: ` + strconv.Itoa(port) + `
username: user
password: test-password
profile: interface-stats
min_collection_interval: 15
`)

	factoryOpt := gnmi.Factory()
	checkFactory, ok := factoryOpt.Get()
	require.True(t, ok)
	checkInstance := checkFactory()

	mockSender := mocksender.NewMockSender(t, "")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = checkInstance.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstance, []byte(""), "test", "test")
	require.NoError(t, err)
	mocksender.SetSender(mockSender, checkInstance.ID())

	assert.Equal(t, gnmi.CheckName, checkInstance.String())

	err = checkInstance.Run()
	require.NoError(t, err)

	event := waitSubscribeEvent(t, server)
	require.NoError(t, server.SendUpdate(event.StreamID, fakeserver.InterfaceInOctetsUpdate("eth0", 42)))
	require.NoError(t, server.SendUpdate(event.StreamID, fakeserver.InterfaceOutOctetsUpdate("eth0", 84)))

	require.Eventually(t, func() bool {
		if err := checkInstance.Run(); err != nil {
			return false
		}
		gotIn := false
		gotOut := false
		for _, call := range mockSender.Calls {
			if call.Method != "MonotonicCount" {
				continue
			}
			switch call.Arguments[0] {
			case "snmp.ifHCInOctets":
				gotIn = true
			case "snmp.ifHCOutOctets":
				gotOut = true
			}
		}
		return gotIn && gotOut
	}, 2*time.Second, 10*time.Millisecond)

	assert.Equal(t, 1, server.ConnectionCount())
	mockSender.AssertCalled(t, "Gauge", "datadog.gnmi.stream_state", mock.Anything, "", mock.Anything)
	mockSender.AssertCalled(t, "Gauge", "datadog.gnmi.received_samples", mock.Anything, "", mock.Anything)
	mockSender.AssertCalled(t, "MonotonicCount", "snmp.ifHCInOctets", float64(42), "", mock.Anything)
	mockSender.AssertCalled(t, "MonotonicCount", "snmp.ifHCOutOctets", float64(84), "", mock.Anything)
	mockSender.AssertCalled(t, "Commit")

	checkInstance.Cancel()
}

func TestCheckRunEmitsHealthMetricsOnlyWhenCacheEmpty(t *testing.T) {
	admission.ResetGateForTesting()
	admission.SetPaceForTesting(10*time.Millisecond, 10)

	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)
	mockConfig.SetInTest("network_devices.gnmi.enabled", true)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", "profiles")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "interface-stats.yaml"), []byte(interfaceStatsProfile), 0o644))

	server, err := fakeserver.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	server.SetExpectedCredentials("user", "test-password")

	host, port, err := hostPort(server.Addr())
	require.NoError(t, err)

	rawInstance := []byte(`
address: ` + host + `
port: ` + strconv.Itoa(port) + `
username: user
password: test-password
profile: interface-stats
min_collection_interval: 15
`)

	factoryOpt := gnmi.Factory()
	checkFactory, ok := factoryOpt.Get()
	require.True(t, ok)
	checkInstance := checkFactory()

	mockSender := mocksender.NewMockSender(t, "")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = checkInstance.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstance, []byte(""), "test", "test")
	require.NoError(t, err)
	mocksender.SetSender(mockSender, checkInstance.ID())

	err = checkInstance.Run()
	require.NoError(t, err)
	waitSubscribeEvent(t, server)

	require.Eventually(t, func() bool {
		if err := checkInstance.Run(); err != nil {
			return false
		}
		for _, call := range mockSender.Calls {
			if call.Method == "Gauge" && call.Arguments[0] == "datadog.gnmi.received_samples" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	mockSender.AssertCalled(t, "Gauge", "datadog.gnmi.stream_state", mock.Anything, "", mock.Anything)
	mockSender.AssertCalled(t, "Gauge", "datadog.gnmi.reconnect_count", mock.Anything, "", mock.Anything)
	mockSender.AssertCalled(t, "Gauge", "datadog.gnmi.received_samples", float64(0), "", mock.Anything)
	mockSender.AssertCalled(t, "Gauge", "datadog.gnmi.sample_age_seconds", float64(0), "", mock.Anything)
	mockSender.AssertNotCalled(t, "MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockSender.AssertCalled(t, "Commit")

	checkInstance.Cancel()
}

func TestConfigureDisabled(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("network_devices.gnmi.enabled", false)

	rawInstance := []byte(`
address: 127.0.0.1
username: user
password: secret
profile: interface-stats
`)

	factoryOpt := gnmi.Factory()
	checkFactory, ok := factoryOpt.Get()
	require.True(t, ok)
	checkInstance := checkFactory()

	mockSender := mocksender.NewMockSender(t, "")

	err := checkInstance.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstance, []byte(""), "test", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network_devices.gnmi.enabled")
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
