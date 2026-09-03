// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package sender

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/stretchr/testify/assert"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

func TestNCMSender_SendNCMConfig_Success(t *testing.T) {
	mockSender := &mocksender.MockSender{}
	namespace := "default"
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))

	ncmSender := NewNCMSender(mockSender, namespace, mockClock, "test-agent-host")

	// Create test payload
	configs := []ncmreport.NetworkDeviceConfig{
		{
			DeviceID:     "default:10.0.0.1",
			DeviceIP:     "10.0.0.1",
			ConfigType:   types.RUNNING,
			ConfigSource: types.CLI,
			Timestamp:    mockClock.Now().Unix(),
			Tags:         []string{"device_ip:10.0.0.1"},
			Content:      "version 15.1\nhostname Router1",
		},
	}

	payload := ncmreport.ToNCMPayload(namespace, "test-agent-host", configs, []ncmreport.InventoryEntry{}, mockClock.Now().Unix())

	// Set up mock expectations
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Once()
	mockSender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	// Send the config
	err := ncmSender.SendNCMPayload(payload)
	assert.NoError(t, err)

	var expectedEvent = []byte(`
{
  "namespace": "default",
  "configs": [
    {
      "device_id": "default:10.0.0.1",
      "device_ip": "10.0.0.1",
      "config_type": "running",
      "config_source": "cli",
      "timestamp": 1754043600,
      "tags": ["device_ip:10.0.0.1"],
      "content": "version 15.1\nhostname Router1"
    }
  ],
  "collect_timestamp": 1754043600,
  "agent_hostname": "test-agent-host"
}
`)

	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, expectedEvent)
	assert.NoError(t, err)
	mockSender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	mockSender.AssertEventPlatformEvent(t, compactEvent.Bytes(), eventplatform.EventTypeNetworkConfigManagement)
	mockSender.AssertExpectations(t)
}

type expectedMetric struct {
	submissionType string
	name           string
	value          float64
	tags           []string
}

func TestNCMSender_SendNCMCheckMetrics(t *testing.T) {
	tests := []struct {
		name            string
		startTime       time.Time
		lastCheckTime   time.Time
		tags            []string
		success         bool
		expectedMetrics []expectedMetric
	}{
		{
			name:          "Submit NCM check metrics successfully",
			startTime:     time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC),
			lastCheckTime: time.Date(2025, 8, 1, 10, 05, 0, 0, time.UTC), // 15 minutes before
			success:       true,
			tags:          []string{"device_ip:10.0.0.1"},
			expectedMetrics: []expectedMetric{
				{
					submissionType: "Gauge",
					name:           ncmCheckDurationMetric,
					value:          5,
					tags:           []string{"device_ip:10.0.0.1", "agent_version:" + version.AgentVersion, "status:ok"},
				},
				{
					submissionType: "Gauge",
					name:           ncmCheckIntervalMetric,
					value:          900, // 15 minutes in seconds
					tags:           []string{"device_ip:10.0.0.1", "agent_version:" + version.AgentVersion, "status:ok"},
				},
			},
		},
		{
			name:      "Last check time is zero (first run), no interval metric sent",
			startTime: time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC),
			success:   true,
			tags:      []string{"device_ip:10.0.0.1"},
			expectedMetrics: []expectedMetric{
				{
					submissionType: "Gauge",
					name:           ncmCheckDurationMetric,
					value:          5,
					tags:           []string{"device_ip:10.0.0.1", "agent_version:" + version.AgentVersion, "status:ok"},
				},
			},
		},
		{
			name:      "Failure",
			startTime: time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC),
			success:   false,
			tags:      []string{"device_ip:10.0.0.1"},
			expectedMetrics: []expectedMetric{
				{
					submissionType: "Gauge",
					name:           ncmCheckDurationMetric,
					value:          5,
					tags:           []string{"device_ip:10.0.0.1", "agent_version:" + version.AgentVersion, "status:error"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(t, "test")
			mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			mockClock := clock.NewMock()
			mockClock.Set(tt.startTime)
			mockClock.Add(5 * time.Second)

			sender := NewNCMSender(mockSender, "test-namespace", mockClock, "test-agent-host")
			sender.SetDeviceTags(tt.tags)

			sender.SendNCMCheckMetrics(tt.startTime, tt.lastCheckTime, tt.success)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, metric.submissionType, metric.name, metric.value, "test-agent-host", metric.tags)
			}
		})
	}
}

func TestNCMSender_SendMetricsFromExtractedMetadata(t *testing.T) {
	tests := []struct {
		name              string
		extractedMetadata profile.ExtractedMetadata
		configType        types.ConfigType
		tags              []string
		expectedMetrics   []expectedMetric
	}{
		{
			name: "Config size metric submitted if present",
			extractedMetadata: profile.ExtractedMetadata{
				ConfigSize: 300,
			},
			configType: ncmRunningConfigTypeTag,
			tags:       []string{"device_id:10.0.0.1", "config_type:running"},
			expectedMetrics: []expectedMetric{
				{
					submissionType: "Gauge",
					name:           ncmConfigSizeMetric,
					value:          300,
				},
			},
		},
		{
			name:            "Nothing extracted - no metrics expected",
			expectedMetrics: []expectedMetric{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(t, "test")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			sender := NewNCMSender(mockSender, "test-namespace", clock.NewMock(), "test-agent-host")
			sender.SetDeviceTags(tt.tags)
			sender.SendMetricsFromExtractedMetadata(tt.extractedMetadata, tt.configType)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, metric.submissionType, metric.name, metric.value, "test-agent-host", tt.tags)
			}
		})
	}
}

func TestNCMSender_SendDeviceMetadata(t *testing.T) {
	mockSender := &mocksender.MockSender{}
	namespace := "test-namespace"
	deviceID := "test-namespace:10.0.0.1"
	deviceIP := "10.0.0.1"
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))

	ncmSender := NewNCMSender(mockSender, namespace, mockClock, "test-agent-host")

	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Once()

	err := ncmSender.SendDeviceMetadata(deviceID, deviceIP)
	assert.NoError(t, err)

	var expectedEvent = []byte(`
{
  "namespace": "test-namespace",
  "integration": "network-configuration-management",
  "devices": [
    {
      "id": "test-namespace:10.0.0.1",
      "id_tags": null,
      "tags": null,
      "ip_address": "10.0.0.1",
      "status": 1
    }
  ],
  "collect_timestamp": 1754043600
}
`)

	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, expectedEvent)
	assert.NoError(t, err)
	mockSender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	mockSender.AssertEventPlatformEvent(t, compactEvent.Bytes(), eventplatform.EventTypeNetworkDevicesMetadata)
	mockSender.AssertExpectations(t)

	// Verify the integration constant value
	assert.Equal(t, integrations.Integration("network-configuration-management"), integrations.NetworkConfigManagement)
}

func TestNCMSender_SendNCMInventory_Success(t *testing.T) {
	mockSender := &mocksender.MockSender{}
	namespace := "default"
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))

	ncmSender := NewNCMSender(mockSender, namespace, mockClock, "test-agent-host")

	payload := ncmreport.NCMPayload{
		Namespace:        namespace,
		AgentHostname:    "test-agent-host",
		CollectTimestamp: mockClock.Now().Unix(),
		Inventories: []ncmreport.InventoryEntry{
			{
				Namespace: "default",
				ConfigID:  "abc-123",
				DeviceID:  "default:10.0.0.1",
			},
		},
	}

	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Once()
	mockSender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	err := ncmSender.SendNCMPayload(payload)
	assert.NoError(t, err)
	expectedEvent := []byte(`
{
  "namespace": "default",
  "inventories": [
    {
      "namespace": "default",
      "config_id": "abc-123",
      "device_id": "default:10.0.0.1"
    }
  ],
  "collect_timestamp": 1754043600,
  "agent_hostname": "test-agent-host"
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, expectedEvent)
	assert.NoError(t, err)

	mockSender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	mockSender.AssertEventPlatformEvent(t, compactEvent.Bytes(), eventplatform.EventTypeNetworkConfigManagement)
	mockSender.AssertMetric(t, "Count", ncmCheckInventoryEntriesSentMetric, 1, "test-agent-host", []string{"agent_version:" + version.AgentVersion})
	mockSender.AssertExpectations(t)
}
