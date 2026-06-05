// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package networkconfigmanagement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	ncmcomp "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmsender "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/sender"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// mockAgentHostname forces hostname.Get() to return the given value during the
// test by injecting it into the mock config and flushing the hostname cache.
func mockAgentHostname(t *testing.T, hostname string) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("hostname", hostname)
	cache.Cache.Delete(cache.BuildAgentKey("hostname"))
	t.Cleanup(func() { cache.Cache.Delete(cache.BuildAgentKey("hostname")) })
}

// Test fixtures and mocks

// MockRemoteClient is a "mocked" Client to help with testing
type MockRemoteClient struct {
	Connection      *MockConnection
	Profile         *profile.NCMProfile
	ConnectionError error
}

var _ ncmremote.Connector = (*MockRemoteClient)(nil)

const (
	runningOutput = `Building configuration...
! Last configuration change at 10:20:00 UTC Fri Aug 1 2025
interface GigabitEthernet0/1
 ip address 192.168.1.1 255.255.255.0`
	startupOutput = `interface GigabitEthernet0/1
ip address 192.168.1.1 255.255.255.0`
	versionOutput = `Cisco Device Version 1.0`
)

func newMockRemoteClient() *MockRemoteClient {
	// Set up mock remote client
	mockClient := &MockRemoteClient{
		Connection: &MockConnection{
			OutputMap: map[string]result{
				"show running-config": ok(runningOutput),
				"show startup-config": ok(startupOutput),
				"show version":        ok(versionOutput),
			},
		},
	}
	return mockClient
}

func (m *MockRemoteClient) Connect() (ncmremote.Connection, error) {
	if m.ConnectionError != nil {
		return nil, m.ConnectionError
	}
	return m.Connection, nil
}

type result struct {
	response string
	err      error
}

func ok(msg string) result {
	return result{response: msg}
}

func fail(err error) result {
	return result{err: err}
}

// MockConnection simulates a Connection
type MockConnection struct {
	OutputMap map[string]result // cmd -> output
	Closed    bool
	Calls     []string
	Profile   *profile.NCMProfile
}

func (m *MockConnection) execute(cmd *profile.PlainCommand) ([]byte, error) {
	r := fail(errors.New("unsupported command"))
	if cmd != nil {
		var ok bool
		r, ok = m.OutputMap[cmd.Command]
		if !ok {
			r = fail(fmt.Errorf("unknown command %q", cmd))
		}
	}
	return []byte(r.response), r.err
}

func (m *MockConnection) RetrieveRunningConfig(_ context.Context) ([]byte, error) {
	return m.execute(m.Profile.Commands.GetRunning)
}

func (m *MockConnection) RetrieveStartupConfig(_ context.Context) ([]byte, error) {
	return m.execute(m.Profile.Commands.GetStartup)
}

func (m *MockConnection) PushConfig(_ context.Context, _ string) error {
	return errors.New("not implemented")
}

func (m *MockConnection) SetProfile(np *profile.NCMProfile) {
	m.Profile = np
}

func (m *MockConnection) Close() error {
	m.Closed = true
	return nil
}

// sequenceUUIDGenerator returns a function that emits the given UUIDs in order
// on successive calls. Useful for making memstore output deterministic in tests.
func sequenceUUIDGenerator(ids ...string) func() string {
	i := 0
	return func() string {
		if i >= len(ids) {
			return fmt.Sprintf("test-uuid-%d", i)
		}
		id := ids[i]
		i++
		return id
	}
}

// Test helper functions

func createTestCheck(t *testing.T) *Check {
	cfg := agentconfig.NewMock(t)
	comp := ncmcomp.Mock(t)
	return newCheck(cfg, comp).(*Check)
}

// Configuration test data

var validConfig = []byte(`
ip_address: 10.0.0.1
profile: p2
auth:
  username: admin
  password: password
  port: "22"
  remote: tcp
`)

var invalidConfigMissingIP = []byte(`
auth:
  username: admin
  password: password
  port: "22"
  remote: tcp
`)

var invalidConfigMissingAuth = []byte(`
ip_address: 10.0.0.1
`)

var baseInitConfig = []byte(`
ssh:
  insecure_skip_verify: true
`)

// Unit Tests

func TestCheck_Configure_ValidConfig(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	profile.SetProfilesForTesting(t, profile.TestProfiles)

	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, baseInitConfig, "test", "provider")

	require.NoError(t, err)
	assert.NotNil(t, check.checkContext)
	assert.Equal(t, "10.0.0.1", check.checkContext.Device.IPAddress)
	assert.Equal(t, "admin", check.checkContext.Device.Auth.Username)
	assert.NotNil(t, check.sender)
	assert.NotNil(t, check.remoteClient)
}

func TestCheck_Configure_InvalidConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        []byte
		expectedError string
	}{
		{
			name:          "missing IP address",
			config:        invalidConfigMissingIP,
			expectedError: "ip_address is required",
		},
		{
			name:          "missing auth",
			config:        invalidConfigMissingAuth,
			expectedError: "auth is required",
		},
		{
			name:          "malformed YAML",
			config:        []byte("invalid: yaml: content:"),
			expectedError: "yaml: mapping values are not allowed in this context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := createTestCheck(t)
			senderManager := mocksender.CreateDefaultDemultiplexer()

			err := check.Configure(senderManager, integration.FakeConfigHash, tt.config, baseInitConfig, "test", "provider")

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestCheck_Run_Success(t *testing.T) {
	check := createTestCheck(t)
	mockAgentHostname(t, "test-agent-host")

	id := checkid.BuildID(CheckName, integration.FakeConfigHash, validConfig, baseInitConfig)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	mockSender := mocksender.NewMockSenderWithSenderManager(id, senderManager)

	// Set up mock sender expectations
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Times(2)
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	// Configure the check
	profile.SetProfilesForTesting(t, profile.TestProfiles)
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, baseInitConfig, "test", "provider")
	require.NoError(t, err)

	// Mock the time and recreate sender with mock clock (Configure creates sender with real clock)
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))
	check.clock = mockClock
	check.sender = ncmsender.NewNCMSender(mockSender, check.checkContext.Namespace, mockClock, check.agentHostname)

	// Swap the ncm component for one backed by a memstore configured with the
	// mock clock and deterministic UUIDs, so inventory output is predictable.
	memStore := ncmstore.NewMemStore(
		ncmstore.WithClock(mockClock),
		ncmstore.WithUUIDGenerator(sequenceUUIDGenerator(
			"87b2343a-56d9-43bc-a35a-4d842dec9586", // running
			"d348e53f-db31-47ed-8d50-11462d7a15e5", // startup
		)),
	)
	check.ncmComp = ncmcomp.MockWithStore(t, memStore)

	// Set up mock remote client
	mockClient := newMockRemoteClient()
	check.remoteClient = mockClient

	// Run the check
	err = check.Run()

	assert.NoError(t, err)
	assert.True(t, mockClient.Connection.Closed, "Remote client should be closed after run")
	expectedTags := []string{
		"device_namespace:default",
		"device_ip:10.0.0.1",
		"device_id:default:10.0.0.1",
		"config_source:cli",
		"profile:p2",
	}
	expectedConfigPayload := report.NCMPayload{
		Namespace: "default",
		Configs: []report.NetworkDeviceConfig{
			{
				DeviceID:     "default:10.0.0.1",
				DeviceIP:     "10.0.0.1",
				ConfigType:   "running",
				ConfigSource: "cli",
				Timestamp:    1754043600,
				Tags:         expectedTags,
				Content:      runningOutput,
				ID:           "87b2343a-56d9-43bc-a35a-4d842dec9586",
				ConfigHash:   hashConfigForTest(runningOutput),
			},
			{
				DeviceID:     "default:10.0.0.1",
				DeviceIP:     "10.0.0.1",
				ConfigType:   "startup",
				ConfigSource: "cli",
				Timestamp:    1754043600, // timestamp taken from agent collection (could not be extracted from config)
				Tags:         expectedTags,
				Content:      startupOutput,
				ID:           "d348e53f-db31-47ed-8d50-11462d7a15e5",
				ConfigHash:   hashConfigForTest(startupOutput),
			},
		},
		Inventories: []report.InventoryEntry{
			{
				Namespace:  "default",
				ConfigID:   "87b2343a-56d9-43bc-a35a-4d842dec9586",
				DeviceID:   "default:10.0.0.1",
				ReportedAt: 1754043600,
			},
			{
				Namespace:  "default",
				ConfigID:   "d348e53f-db31-47ed-8d50-11462d7a15e5",
				DeviceID:   "default:10.0.0.1",
				ReportedAt: 1754043600,
			},
		},
		CollectTimestamp: 1754043600,
		AgentHostname:    "test-agent-host",
	}
	expectedConfigEvent, err := json.Marshal(expectedConfigPayload)
	assert.NoError(t, err)

	// Build expected device metadata payload
	expectedDeviceMetadataPayload := devicemetadata.NetworkDevicesMetadata{
		Namespace:   "default",
		Integration: integrations.NetworkConfigManagement,
		Devices: []devicemetadata.DeviceMetadata{
			{
				ID:        "default:10.0.0.1",
				IPAddress: "10.0.0.1",
				Status:    devicemetadata.DeviceStatusReachable,
			},
		},
		CollectTimestamp: 1754043600,
	}
	expectedDeviceMetadata, err := json.Marshal(expectedDeviceMetadataPayload)
	assert.NoError(t, err)

	mockSender.AssertNumberOfCalls(t, "EventPlatformEvent", 2)
	mockSender.AssertEventPlatformEvent(t, expectedConfigEvent, eventplatform.EventTypeNetworkConfigManagement)

	// ID is randomly generated per store call, so unmarshal the actual NCM
	// payload, capture the IDs, and assert the rest of the payload matches.
	actualNCMPayload := findEventPlatformEventPayload(t, mockSender, eventplatform.EventTypeNetworkConfigManagement)
	require.Len(t, actualNCMPayload.Configs, len(expectedConfigPayload.Configs))
	uuidRe := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	for i := range actualNCMPayload.Configs {
		assert.Regexp(t, uuidRe, actualNCMPayload.Configs[i].ID, "config[%d] should have a valid UUID", i)
		expectedConfigPayload.Configs[i].ID = actualNCMPayload.Configs[i].ID
	}
	assert.Equal(t, expectedConfigPayload, actualNCMPayload)

	mockSender.AssertEventPlatformEvent(t, expectedDeviceMetadata, eventplatform.EventTypeNetworkDevicesMetadata)
	mockSender.AssertMetricTaggedWith(t, "Gauge", "datadog.ncm.check_duration", expectedTags)
	mockSender.AssertMetric(t, "Count", "datadog.ncm.inventory.entries_sent", 2, "test-agent-host", []string{"agent_host:test-agent-host"})
	mockSender.AssertExpectations(t)
}

// hashConfigForTest mirrors the SHA-256 hashing done by the config store, so tests
// can predict the ConfigHash field of stored configs without depending on the store package.
func hashConfigForTest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// findEventPlatformEventPayload returns the unmarshalled NCMPayload from the mock sender's
// EventPlatformEvent call matching the given event type. Fails the test if no matching call exists.
func findEventPlatformEventPayload(t *testing.T, m *mocksender.MockSender, eventType string) report.NCMPayload {
	t.Helper()
	for _, call := range m.Mock.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		if got, _ := call.Arguments[1].(string); got != eventType {
			continue
		}
		raw, _ := call.Arguments[0].([]byte)
		var payload report.NCMPayload
		require.NoError(t, json.Unmarshal(raw, &payload))
		return payload
	}
	t.Fatalf("no EventPlatformEvent call found for event type %s", eventType)
	return report.NCMPayload{}
}

func TestCheck_Run_ConnectionFailure(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure the check
	profile.SetProfilesForTesting(t, profile.TestProfiles)
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, baseInitConfig, "test", "provider")
	require.NoError(t, err)

	// Set up mock remote client factory that fails to connect
	client := newMockRemoteClient()
	client.ConnectionError = errors.New("connection refused")
	check.remoteClient = client

	// Run the check
	err = check.Run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestCheck_Run_ConfigRetrievalFailure_NoProfileMatch(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure the check
	profile.SetProfilesForTesting(t, profile.TestProfiles)
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, baseInitConfig, "test", "provider")
	require.NoError(t, err)

	// Clear the profile so FindMatchingProfile is exercised
	check.checkContext.ProfileCache.ProfileName = ""
	check.checkContext.ProfileCache.Profile = nil

	// Set up a mock remote client that fails config retrieval
	mockClient := newMockRemoteClient()
	mockClient.Connection.OutputMap["show running-config"] = fail(errors.New("command execution failed"))

	check.remoteClient = mockClient

	// Run the check
	err = check.Run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to find matching profile for device 10.0.0.1")
	assert.True(t, mockClient.Connection.Closed, "Remote client should be closed even on failure")
}

func TestCheck_FindMatchingProfile(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure the check
	profile.SetProfilesForTesting(t, profile.TestProfiles)
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, baseInitConfig, "test", "provider")
	require.NoError(t, err)

	// mock the time
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))
	check.clock = mockClock

	// Set up mock remote client
	mockClient := newMockRemoteClient()
	check.remoteClient = mockClient
	conn, err := mockClient.Connect()
	require.NoError(t, err)

	// Run the profile matching function
	// Expected that the `_base` profile will fail and still continue to the next one (p2)
	actual, err := check.FindMatchingProfile(conn)
	assert.NoError(t, err)

	expected := profile.TestProfiles["p2"]
	assert.Equal(t, expected, actual)
}

func TestCheck_FindMatchingProfile_Error(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure the check
	profile.SetProfilesForTesting(t, profile.TestProfiles)
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, baseInitConfig, "test", "provider")
	require.NoError(t, err)

	// mock the time
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))
	check.clock = mockClock

	// Set up mock remote client, remove the version command for the test to fail
	mockClient := newMockRemoteClient()
	check.remoteClient = mockClient
	delete(mockClient.Connection.OutputMap, "show running-config")
	conn, err := mockClient.Connect()
	require.NoError(t, err)

	// Run the profile matching function
	_, err = check.FindMatchingProfile(conn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to find matching profile for device")
}
