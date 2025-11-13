// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package sender

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	"github.com/stretchr/testify/assert"
)

func TestNCMSender_SendNCMConfig_Success(t *testing.T) {
	mockSender := &mocksender.MockSender{}
	namespace := "default"
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))

	ncmSender := NewNCMSender(mockSender, namespace, mockClock)

	// Create test payload
	configs := []ncmreport.NetworkDeviceConfig{
		{
			DeviceID:     "default:10.0.0.1",
			DeviceIP:     "10.0.0.1",
			ConfigType:   string(ncmreport.RUNNING),
			ConfigSource: string(ncmreport.CLI),
			Timestamp:    mockClock.Now().Unix(),
			Tags:         []string{"device_ip:10.0.0.1"},
			Content:      "version 15.1\nhostname Router1",
		},
	}

	payload := ncmreport.ToNCMPayload(namespace, configs, mockClock.Now().Unix())

	// Set up mock expectations
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Once()

	// Send the config
	err := ncmSender.SendNCMConfig(payload)
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
  "collect_timestamp": 1754043600
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
		expectedMetrics []expectedMetric
	}{
		{
			name:          "Submit NCM check metrics successfully",
			startTime:     time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC),
			lastCheckTime: time.Date(2025, 8, 1, 10, 05, 0, 0, time.UTC), // 15 minutes before
			tags:          []string{"device_ip:10.0.0.1"},
			expectedMetrics: []expectedMetric{
				{
					submissionType: "Gauge",
					name:           ncmCheckDurationMetric,
					value:          5,
					tags:           []string{"device_ip:10.0.0.1", "agent_version:" + version.AgentVersion},
				},
				{
					submissionType: "Gauge",
					name:           ncmCheckIntervalMetric,
					value:          900, // 15 minutes in seconds
					tags:           []string{"device_ip:10.0.0.1", "agent_version:" + version.AgentVersion},
				},
			},
		},
		{
			name:      "Last check time is zero (first run), no interval metric sent",
			startTime: time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC),
			tags:      []string{"device_ip:10.0.0.1"},
			expectedMetrics: []expectedMetric{
				{
					submissionType: "Gauge",
					name:           ncmCheckDurationMetric,
					value:          5,
					tags:           []string{"device_ip:10.0.0.1", "agent_version:" + version.AgentVersion},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("test")
			mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			mockClock := clock.NewMock()
			mockClock.Set(tt.startTime)
			mockClock.Add(5 * time.Second)

			sender := NewNCMSender(mockSender, "test-namespace", mockClock)
			sender.SetDeviceTags(tt.tags)

			sender.SendNCMCheckMetrics(tt.startTime, tt.lastCheckTime)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, metric.submissionType, metric.name, metric.value, "", metric.tags)
			}
		})
	}
}

func TestNCMSender_SendMetricsFromExtractedMetadata(t *testing.T) {
	tests := []struct {
		name              string
		extractedMetadata profile.ExtractedMetadata
		configType        ncmreport.ConfigType
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
			mockSender := mocksender.NewMockSender("test")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			sender := NewNCMSender(mockSender, "test-namespace", clock.NewMock())
			sender.SetDeviceTags(tt.tags)
			sender.SendMetricsFromExtractedMetadata(tt.extractedMetadata, tt.configType)

			for _, metric := range tt.expectedMetrics {
				mockSender.AssertMetric(t, metric.submissionType, metric.name, metric.value, "", tt.tags)
			}
		})
	}
}
