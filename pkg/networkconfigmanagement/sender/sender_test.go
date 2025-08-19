// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package sender

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	"github.com/stretchr/testify/assert"
)

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
      "content": "version 15.1\nhostname Router1"
    }
  ],
  "collect_timestamp": 1754043720
}
`)

func TestNCMSender_SendNCMConfig_Success(t *testing.T) {
	mockSender := &mocksender.MockSender{}
	namespace := "default"
	ncmSender := NewNCMSender(mockSender, namespace)

	// Create test payload
	configs := []ncmreport.NetworkDeviceConfig{
		{
			DeviceID:   "default:10.0.0.1",
			DeviceIP:   "10.0.0.1",
			ConfigType: string(ncmreport.RUNNING),
			Timestamp:  mockTimeNow().Unix(),
			Tags:       []string{"device_ip:10.0.0.1"},
			Content:    "version 15.1\nhostname Router1",
		},
	}

	payload := ncmreport.ToNCMPayload(namespace, "", configs, mockTimeNow().Unix())

	// Set up mock expectations
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Once()

	// Send the config
	err := ncmSender.SendNCMConfig(payload)
	assert.NoError(t, err)

	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, expectedEvent)
	assert.NoError(t, err)
	mockSender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	mockSender.AssertEventPlatformEvent(t, compactEvent.Bytes(), eventplatform.EventTypeNetworkConfigManagement)
	mockSender.AssertExpectations(t)
}
