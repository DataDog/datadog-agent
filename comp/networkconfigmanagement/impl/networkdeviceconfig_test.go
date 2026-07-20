// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

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

	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// Test fixtures and mocks
const (
	runningOutput = `Building configuration...
! Last configuration change at 10:20:00 UTC Fri Aug 1 2025
interface GigabitEthernet0/1
 ip address 192.168.1.1 255.255.255.0`
	startupOutput = `interface GigabitEthernet0/1
ip address 192.168.1.1 255.255.255.0`
	versionOutput = `Cisco Device Version 1.0`
)

type result = ncmremote.CommandResult

func ok(msg string) *result {
	return &result{Output: msg}
}

func fail(err error) *result {
	return &result{Error: err}
}

func newMockConnection() *MockConnection {
	// Set up mock remote client
	return &MockConnection{
		OutputMap: map[string]*result{
			"show running-config": ok(runningOutput),
			"show startup-config": ok(startupOutput),
			"show version":        ok(versionOutput),
			"show system":         ok("Test System"),
		},
	}
}

// MockConnection simulates a Connection
type MockConnection struct {
	OutputMap map[string]*result // cmd -> output
	Opened    bool
	Closed    bool
	Calls     []string
	Profile   *profile.NCMProfile
}

var _ ncmremote.Connection = (*MockConnection)(nil)

func (m *MockConnection) execute(cmd *profile.PlainCommand) (*ncmremote.CommandResult, error) {
	r := fail(errors.New("unsupported command"))
	if cmd != nil {
		var ok bool
		r, ok = m.OutputMap[cmd.Command]
		if !ok {
			r = fail(fmt.Errorf("unknown command %q", cmd.Command))
		}
		r.CommandStr = cmd.Command
		if r.Error == nil {
			r.Error = cmd.Validator.Validate(r.Output)
		}
	}
	return r, r.FormattedError()
}

func (m *MockConnection) RetrieveRunningConfig(_ context.Context) (*result, error) {
	return m.execute(m.Profile.Commands.GetRunning)
}

func (m *MockConnection) RetrieveStartupConfig(_ context.Context) (*result, error) {
	return m.execute(m.Profile.Commands.GetStartup)
}

func (m *MockConnection) Verify(_ context.Context) error {
	_, err := m.execute(m.Profile.Commands.Verify)
	return err
}

func (m *MockConnection) PushConfig(_ context.Context, _ string) (*ncmremote.PushResult, error) {
	return nil, errors.New("not implemented")
}

func (m *MockConnection) SetProfile(np *profile.NCMProfile) {
	m.Profile = np
}

func (m *MockConnection) Close() error {
	m.Closed = true
	return nil
}

type MockConnFactory struct {
	connectionError error
	conn            *MockConnection
}

func (m *MockConnFactory) Connect(_ *ncmconfig.DeviceInstance) (ncmremote.Connection, error) {
	if m.connectionError != nil {
		return nil, m.connectionError
	}
	m.conn.Opened = true
	return m.conn, nil
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

type services struct {
	config      agentconfig.Component
	sender      *mocksender.MockSender
	store       ncmstore.ConfigStore
	logger      log.Component
	clock       *clock.Mock
	connFactory *MockConnFactory
}

// Test helper functions
func createTestComponent(t *testing.T) (*networkDeviceConfigImpl, *services) {
	t.Helper()

	// we need this because the NCMSender gets tags from
	// pkg/networkdevice/utils, which queries for the hostname directly instead
	// of using the hostname service component.
	cfg := configmock.New(t)
	cfg.SetInTest("hostname", "test-agent-host")
	cache.Cache.Delete(cache.BuildAgentKey("hostname"))
	t.Cleanup(func() { cache.Cache.Delete(cache.BuildAgentKey("hostname")) })

	conf := agentconfig.NewMock(t)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	sender := mocksender.NewMockSenderWithSenderManager(CheckName, senderManager)
	clock := clock.NewMock()
	clock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))
	logger := logmock.New(t)

	store := ncmstore.NewMemStore(
		ncmstore.WithClock(clock),
		ncmstore.WithUUIDGenerator(sequenceUUIDGenerator(
			"87b2343a-56d9-43bc-a35a-4d842dec9586", // running
			"d348e53f-db31-47ed-8d50-11462d7a15e5", // startup
		)),
	)

	reqs := &services{
		config: conf,
		sender: sender,
		store:  store,
		logger: logger,
		clock:  clock,
		connFactory: &MockConnFactory{
			conn: newMockConnection(),
		},
	}

	return newNetworkDeviceConfigImpl(
		reqs.logger,
		reqs.store,
		reqs.sender,
		"test-agent-host",
		profile.TestProfiles,
		reqs.connFactory.Connect,
		reqs.clock,
	), reqs
}

func createTestDevice() *ncmconfig.DeviceInstance {
	return &ncmconfig.DeviceInstance{
		IPAddress: "10.0.0.1",
		Namespace: "default",
		Profile:   "p2",
		Auth: ncmconfig.AuthCredentials{
			Username: "admin",
			Password: "password",
			Port:     "22",
			Protocol: "tcp",
		},
	}
}

func TestCheck_Run_Success(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestDevice()
	err := comp.RegisterDevice(device)
	assert.NoError(t, err)

	mockSender := reqs.sender
	// Set up mock sender expectations
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Times(2)
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = comp.ReportConfig(t.Context(), device.DeviceID(), reqs.sender)
	assert.NoError(t, err)
	assert.True(t, reqs.connFactory.conn.Closed, "Remote client should be closed after run")
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
				DeviceID:      "default:10.0.0.1",
				DeviceIP:      "10.0.0.1",
				ConfigType:    "running",
				ConfigSource:  "cli",
				ConfigProfile: "p2",
				Timestamp:     1754043600,
				Tags:          expectedTags,
				Content:       runningOutput,
				ID:            "87b2343a-56d9-43bc-a35a-4d842dec9586",
				ConfigHash:    hashConfigForTest(runningOutput),
			},
			{
				DeviceID:      "default:10.0.0.1",
				DeviceIP:      "10.0.0.1",
				ConfigType:    "startup",
				ConfigSource:  "cli",
				ConfigProfile: "p2",
				Timestamp:     1754043600, // timestamp taken from agent collection (could not be extracted from config)
				Tags:          expectedTags,
				Content:       startupOutput,
				ID:            "d348e53f-db31-47ed-8d50-11462d7a15e5",
				ConfigHash:    hashConfigForTest(startupOutput),
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
	comp, reqs := createTestComponent(t)
	reqs.connFactory.connectionError = errors.New("connection refused")

	device := createTestDevice()
	err := comp.RegisterDevice(device)
	assert.NoError(t, err)

	err = comp.ReportConfig(t.Context(), device.DeviceID(), reqs.sender)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestCheck_Run_ConfigRetrievalFailure_NoProfileMatch(t *testing.T) {
	comp, reqs := createTestComponent(t)

	device := createTestDevice()
	device.Profile = ""
	err := comp.RegisterDevice(device)
	assert.NoError(t, err)
	dc, err := comp.devices.Get(device.DeviceID())
	assert.NoError(t, err)
	assert.Nil(t, dc.profile)

	err = comp.ReportConfig(t.Context(), device.DeviceID(), reqs.sender)
	assert.ErrorContains(t, err, "no matching NCM profile for device default:10.0.0.1")
	assert.Nil(t, dc.profile)
	assert.True(t, reqs.connFactory.conn.Closed, "Remote client should be closed even on failure")
}

func TestCheck_Run_ConfigRetrievalFailure_BadProfile(t *testing.T) {
	comp, reqs := createTestComponent(t)
	device := createTestDevice()
	device.Profile = "not-a-profile"
	err := comp.RegisterDevice(device)
	assert.ErrorContains(t, err, "nonexistent NCM profile \"not-a-profile\" specified for device default:10.0.0.1")

	err = comp.ReportConfig(t.Context(), device.DeviceID(), reqs.sender)
	assert.ErrorContains(t, err, "unknown device", "Device should not be registered if profile lookup failed.")
	assert.False(t, reqs.connFactory.conn.Opened, "Remote client should not be opened if config is faulty")
}

func TestCheck_Run_ProfileMatch(t *testing.T) {
	comp, reqs := createTestComponent(t)
	reqs.connFactory.conn.OutputMap["show system"] = ok("OS: System P2.1")
	device := createTestDevice()
	device.Profile = ""
	err := comp.RegisterDevice(device)
	assert.NoError(t, err)
	dc, err := comp.devices.Get(device.DeviceID())
	assert.NoError(t, err)
	assert.Nil(t, dc.profile)

	mockSender := reqs.sender
	// Set up mock sender expectations
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Times(2)
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = comp.ReportConfig(t.Context(), device.DeviceID(), reqs.sender)
	assert.NoError(t, err)
	assert.True(t, reqs.connFactory.conn.Closed)

	if assert.NotNil(t, dc.profile) {
		assert.Equal(t, "p2", string(dc.profile.Name), "Device profile should be detected as p2")
	}

	t.Run("reloading config resets detected profile", func(t *testing.T) {
		err := comp.RegisterDevice(device)
		assert.NoError(t, err)
		assert.Nil(t, dc.profile)
	})
}

func TestCheck_FindMatchingProfile(t *testing.T) {
	comp, reqs := createTestComponent(t)
	reqs.connFactory.conn.OutputMap["show system"] = ok("OS: System P2.1")
	device := createTestDevice()
	err := comp.RegisterDevice(device)
	assert.NoError(t, err)

	conn, err := reqs.connFactory.Connect(device)
	require.NoError(t, err)

	// Run the profile matching function
	actual, ok := comp.findMatchingProfile(t.Context(), conn)
	assert.True(t, ok)
	if assert.NotNil(t, actual) {
		assert.Equal(t, "p2", string(actual.Name))
	}
}

func TestCheck_FindMatchingProfile_Failure(t *testing.T) {
	comp, reqs := createTestComponent(t)
	reqs.connFactory.conn.OutputMap["show running-config"] = fail(errors.New("command execution failed"))
	device := createTestDevice()
	err := comp.RegisterDevice(device)
	assert.NoError(t, err)

	// Remove the version command for the test to fail
	conn, err := reqs.connFactory.Connect(device)
	require.NoError(t, err)

	// Run the profile matching function
	_, ok := comp.findMatchingProfile(t.Context(), conn)
	assert.False(t, ok)
}
