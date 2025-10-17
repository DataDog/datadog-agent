// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package networkconfigmanagement

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
)

// Test fixtures and mocks

// MockRemoteClient is a "mocked" Client to help with testing
type MockRemoteClient struct {
	Session         *MockRemoteSession
	Profile         *profile.NCMProfile
	Closed          bool
	RunningConfig   string
	StartupConfig   string
	ConnectionError error
	ConfigError     error
}

const (
	runningOutput = "Building configuration...\n! Last configuration change at 10:20:00 UTC Fri Aug 1 2025\ninterface GigabitEthernet0/1\n ip address 192.168.1.1 255.255.255.0"
	startupOutput = "interface GigabitEthernet0/1\n ip address 192.168.1.1 255.255.255.0"
	versionOutput = "Cisco Device Version 1.0"
)

func newMockRemoteClient() *MockRemoteClient {
	// Set up mock remote client
	mockClient := &MockRemoteClient{
		Session: &MockRemoteSession{
			OutputMap: map[string]string{
				"show running-config": runningOutput,
				"show startup-config": startupOutput,
				"show version":        versionOutput,
			},
		},
	}
	return mockClient
}

func (m *MockRemoteClient) Connect() error {
	return m.ConnectionError
}

func (m *MockRemoteClient) NewSession() (ncmremote.Session, error) {
	if m.ConnectionError != nil {
		return nil, m.ConnectionError
	}
	return m.Session, nil
}

func (m *MockRemoteClient) RetrieveRunningConfig() ([]byte, error) {
	if m.ConfigError != nil {
		return []byte{}, m.ConfigError
	}
	runningCommand, err := m.Profile.GetCommandValues(profile.Running)
	if err != nil {
		return []byte{}, err
	}
	output, err := m.Session.CombinedOutput(runningCommand[0])
	return output, err
}

func (m *MockRemoteClient) RetrieveStartupConfig() ([]byte, error) {
	if m.ConfigError != nil {
		return []byte{}, m.ConfigError
	}
	runningCommand, err := m.Profile.GetCommandValues(profile.Startup)
	if err != nil {
		return []byte{}, err
	}
	output, err := m.Session.CombinedOutput(runningCommand[0])
	return output, err
}

func (m *MockRemoteClient) SetProfile(np *profile.NCMProfile) {
	m.Profile = np
}

func (m *MockRemoteClient) Close() error {
	m.Closed = true
	return nil
}

// MockRemoteSession simulates a Session
type MockRemoteSession struct {
	OutputMap map[string]string // cmd -> output
	Closed    bool
	Calls     []string
}

func (m *MockRemoteSession) CombinedOutput(cmd string) ([]byte, error) {
	m.Calls = append(m.Calls, cmd)

	if output, ok := m.OutputMap[cmd]; ok {
		return []byte(output), nil
	}
	return []byte(""), fmt.Errorf("no such command: %s", cmd)
}

func (m *MockRemoteSession) Close() error {
	m.Closed = true
	return nil
}

// Test helper functions

func createTestCheck(t *testing.T) *Check {
	cfg := agentconfig.NewMock(t)
	return newCheck(cfg).(*Check)
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

// Unit Tests

func TestCheck_Configure_ValidConfig(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	profile.SetConfdPathAndCleanProfiles()
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, []byte{}, "test")

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

			err := check.Configure(senderManager, integration.FakeConfigHash, tt.config, []byte{}, "test")

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestCheck_Run_Success(t *testing.T) {
	check := createTestCheck(t)

	id := checkid.BuildID(CheckName, integration.FakeConfigHash, validConfig, []byte(``))
	senderManager := mocksender.CreateDefaultDemultiplexer()
	mockSender := mocksender.NewMockSenderWithSenderManager(id, senderManager)

	// Set up mock sender expectations
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Once()
	mockSender.On("Commit").Return()

	// Configure the check
	profile.SetConfdPathAndCleanProfiles()
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, []byte{}, "test")
	require.NoError(t, err)

	// mock the time
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))
	check.clock = mockClock

	// Set up mock remote client
	mockClient := newMockRemoteClient()
	check.remoteClient = mockClient

	// Run the check
	err = check.Run()

	assert.NoError(t, err)
	assert.True(t, mockClient.Closed, "Remote client should be closed after run")

	runningBytes, _ := json.Marshal([]byte(runningOutput))
	startupBytes, _ := json.Marshal([]byte(startupOutput))

	var expectedEvent = []byte(fmt.Sprintf(`
{
  "namespace": "default",
  "integration": "",
  "configs": [
    {
      "device_id": "default:10.0.0.1",
      "device_ip": "10.0.0.1",
      "config_type": "running",
      "timestamp": 1754043600,
      "tags": ["device_ip:10.0.0.1"],
      "content": %s
    },
    {
      "device_id": "default:10.0.0.1",
      "device_ip": "10.0.0.1",
      "config_type": "startup",
      "timestamp": 0,
      "tags": ["device_ip:10.0.0.1"],
      "content": %s
    }
  ],
  "collect_timestamp": 1754043600
}
`, runningBytes, startupBytes))

	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, expectedEvent)
	assert.NoError(t, err)
	mockSender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	mockSender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "ndmconfig")
	mockSender.AssertExpectations(t)
}

func TestCheck_Run_ConnectionFailure(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure the check
	profile.SetConfdPathAndCleanProfiles()
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, []byte{}, "test")
	require.NoError(t, err)

	// Set up mock remote client factory that fails to connect
	connectionError := fmt.Errorf("connection refused")
	client := newMockRemoteClient()
	client.ConnectionError = connectionError

	// Run the check
	err = client.Connect()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestCheck_Run_ConfigRetrievalFailure(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure the check
	profile.SetConfdPathAndCleanProfiles()
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, []byte{}, "test")
	require.NoError(t, err)

	// Set up a mock remote client that fails config retrieval
	mockClient := &MockRemoteClient{
		ConfigError: fmt.Errorf("command execution failed"),
	}
	check.remoteClient = mockClient

	// Run the check
	err = check.Run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command execution failed")
	assert.True(t, mockClient.Closed, "Remote client should be closed even on failure")
}

func TestCheck_FindMatchingProfile(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure the check
	profile.SetConfdPathAndCleanProfiles()
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, []byte{}, "test")
	require.NoError(t, err)

	// mock the time
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))
	check.clock = mockClock

	// Set up mock remote client
	mockClient := newMockRemoteClient()
	check.remoteClient = mockClient

	// Run the profile matching function
	// Expected that the `_base` profile will fail and still continue to the next one (p2)
	actual, err := check.FindMatchingProfile()
	assert.NoError(t, err)

	expected := &profile.NCMProfile{
		BaseProfile: profile.BaseProfile{
			Name: "p2",
		},
		Commands: map[profile.CommandType]profile.Commands{
			profile.Running: {
				CommandType: profile.Running,
				Values:      []string{"show running-config"},
				ProcessingRules: profile.ProcessingRules{
					MetadataRules: []profile.MetadataRule{
						{
							Type:   profile.Timestamp,
							Regex:  `! Last configuration change at (.*)`,
							Format: "15:04:05 MST Mon Jan 2 2006",
						},
						{
							Type:  profile.ConfigSize,
							Regex: `Current configuration : (?P<Size>\d+)`,
						},
					},
					ValidationRules: []profile.ValidationRule{
						{
							Type:    "valid_output",
							Pattern: "Building configuration...",
						},
					},
					RedactionRules: []profile.RedactionRule{
						{Regex: `(username .+ (password|secret) \d) .+`, Replacement: "<redacted secret>"},
					},
				},
			},
			profile.Startup: {
				CommandType: profile.Startup,
				Values:      []string{"show startup-config"},
			},
			profile.Version: {
				CommandType: profile.Version,
				Values:      []string{"show version"},
			},
		},
		Scrubber: scrubber.New(),
	}
	assert.Equal(t, expected, actual)
}

func TestCheck_FindMatchingProfile_Error(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	// Configure the check
	profile.SetConfdPathAndCleanProfiles()
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, []byte{}, "test")
	require.NoError(t, err)

	// mock the time
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))
	check.clock = mockClock

	// Set up mock remote client, remove the version command for the test to fail
	mockClient := newMockRemoteClient()
	check.remoteClient = mockClient
	delete(mockClient.Session.OutputMap, "show running-config")

	// Run the profile matching function
	_, err = check.FindMatchingProfile()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to find matching profile for device")
}
