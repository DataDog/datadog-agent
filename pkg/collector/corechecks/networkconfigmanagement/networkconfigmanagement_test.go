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
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
)

// Test fixtures and mocks

// MockRemoteClient is a "mocked" Client to help with testing
type MockRemoteClient struct {
	Session         *MockRemoteSession
	Closed          bool
	RunningConfig   string
	StartupConfig   string
	ConnectionError error
	ConfigError     error
}

func newMockRemoteClient() *MockRemoteClient {
	// Set up mock remote client
	mockClient := &MockRemoteClient{
		Session: &MockRemoteSession{
			OutputMap: map[string]string{
				"show running-config": "interface GigabitEthernet0/1\n ip address 192.168.1.1 255.255.255.0",
				"show startup-config": "interface GigabitEthernet0/1\n ip address 192.168.1.1 255.255.255.0",
			},
		},
		RunningConfig: "interface GigabitEthernet0/1\n ip address 192.168.1.1 255.255.255.0",
		StartupConfig: "interface GigabitEthernet0/1\n ip address 192.168.1.1 255.255.255.0",
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

func (m *MockRemoteClient) RetrieveRunningConfig() (string, error) {
	if m.ConfigError != nil {
		return "", m.ConfigError
	}
	return m.RunningConfig, nil
}

func (m *MockRemoteClient) RetrieveStartupConfig() (string, error) {
	if m.ConfigError != nil {
		return "", m.ConfigError
	}
	return m.StartupConfig, nil
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

// mockTimeNow mocks time.Now
var mockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2025-08-01 10:22:00"
	t, _ := time.Parse(layout, str)
	return t
}

// language=json
var expectedEvent = []byte(`
{
  "namespace": "default",
  "integration": "",
  "configs": [
    {
      "device_id": "default:10.0.0.1",
      "device_ip": "10.0.0.1",
      "config_type": "running",
      "timestamp": 1754043720,
      "tags": ["device_ip:10.0.0.1"],
      "content": "interface GigabitEthernet0/1\n ip address 192.168.1.1 255.255.255.0"
    }
  ],
  "collect_timestamp": 1754043720
}
`)

// Unit Tests

func TestCheck_Configure_ValidConfig(t *testing.T) {
	check := createTestCheck(t)
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, []byte{}, "test")

	require.NoError(t, err)
	assert.NotNil(t, check.deviceConfig)
	assert.Equal(t, "10.0.0.1", check.deviceConfig.IPAddress)
	assert.Equal(t, "admin", check.deviceConfig.Auth.Username)
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
	TimeNow = mockTimeNow

	check := createTestCheck(t)

	id := checkid.BuildID(CheckName, integration.FakeConfigHash, validConfig, []byte(``))
	senderManager := mocksender.CreateDefaultDemultiplexer()
	mockSender := mocksender.NewMockSenderWithSenderManager(id, senderManager)

	// Set up mock sender expectations
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Once()
	mockSender.On("Commit").Return()

	// Configure the check
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, []byte{}, "test")
	require.NoError(t, err)

	// Set up mock remote client
	mockClient := newMockRemoteClient()
	check.remoteClient = mockClient

	// Run the check
	err = check.Run()

	assert.NoError(t, err)
	assert.True(t, mockClient.Closed, "Remote client should be closed after run")

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
	err := check.Configure(senderManager, integration.FakeConfigHash, validConfig, []byte{}, "test")
	require.NoError(t, err)

	// Set up mock remote client that fails config retrieval
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
