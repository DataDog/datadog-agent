// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package sender

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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
	ncmSender := NewNCMSender(mockSender, namespace)

	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2025, 8, 1, 10, 20, 0, 0, time.UTC))

	// Create test payload
	configs := []ncmreport.NetworkDeviceConfig{
		{
			DeviceID:   "default:10.0.0.1",
			DeviceIP:   "10.0.0.1",
			ConfigType: string(ncmreport.RUNNING),
			Timestamp:  mockClock.Now().Unix(),
			Tags:       []string{"device_ip:10.0.0.1"},
			Content:    []byte("version 15.1\nhostname Router1"),
		},
	}

	payload := ncmreport.ToNCMPayload(namespace, configs, mockClock.Now().Unix())

	// Set up mock expectations
	mockSender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Once()

	// Send the config
	err := ncmSender.SendNCMConfig(payload)
	assert.NoError(t, err)

	contentStr := "version 15.1\nhostname Router1"
	contentBytes, _ := json.Marshal([]byte(contentStr))

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
    }
  ],
  "collect_timestamp": 1754043600
}
`, contentBytes))

	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, expectedEvent)
	assert.NoError(t, err)
	mockSender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	mockSender.AssertEventPlatformEvent(t, compactEvent.Bytes(), eventplatform.EventTypeNetworkConfigManagement)
	mockSender.AssertExpectations(t)
}
