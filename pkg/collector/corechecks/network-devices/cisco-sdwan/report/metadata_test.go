// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package report

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// mockTimeNow mocks time.Now
var mockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

func TestSendMetadata(t *testing.T) {
	TimeNow = mockTimeNow

	sender := mocksender.NewMockSender("testID") // required to initiate aggregator
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	ms := NewSDWanSender(sender, "my-ns")

	devices := []devicemetadata.DeviceMetadata{
		{
			ID:           "my-ns:10.0.0.1",
			IPAddress:    "10.0.0.1",
			Vendor:       "cisco",
			Tags:         []string{"source:cisco-sdwan", "site_id:100"},
			IDTags:       []string{"system_ip:10.0.0.1"},
			Status:       devicemetadata.DeviceStatusReachable,
			Model:        "c8000",
			OsName:       "IOS",
			Version:      "17.12",
			SerialNumber: "NDMTESTSERIAL",
			DeviceType:   "router",
			ProductName:  "c8000",
			Location:     "my-location",
			Integration:  "cisco-sdwan",
		},
	}

	interfaces := []devicemetadata.InterfaceMetadata{
		{
			DeviceID:    "my-ns:10.0.0.1",
			IDTags:      []string{"interface:test-interface"},
			Index:       0,
			Name:        "test-interface",
			Description: "test interface",
			MacAddress:  "11:22:33:44",
			OperStatus:  devicemetadata.OperStatusUp,
			AdminStatus: devicemetadata.AdminStatusUp,
		},
	}

	ipAddresses := []devicemetadata.IPAddressMetadata{
		{
			InterfaceID: "my-ns:10.0.0.1:0",
			IPAddress:   "10.1.1.1",
			Prefixlen:   24,
		},
	}

	ms.SendMetadata(devices, interfaces, ipAddresses)

	// language=json
	event := []byte(`
{
  "namespace": "my-ns",
  "devices": [
    {
      "id": "my-ns:10.0.0.1",
      "id_tags": [
        "system_ip:10.0.0.1"
      ],
      "tags": [
        "source:cisco-sdwan",
        "site_id:100"
      ],
      "ip_address": "10.0.0.1",
      "status": 1,
      "location": "my-location",
      "vendor": "cisco",
      "serial_number": "NDMTESTSERIAL",
      "version": "17.12",
      "product_name": "c8000",
      "model": "c8000",
      "os_name": "IOS",
      "integration": "cisco-sdwan",
      "device_type": "router"
    }
  ],
  "interfaces": [
    {
      "device_id": "my-ns:10.0.0.1",
      "id_tags": [
        "interface:test-interface"
      ],
      "index": 0,
      "name": "test-interface",
      "description": "test interface",
      "mac_address": "11:22:33:44",
      "admin_status": 1,
      "oper_status": 1
    }
  ],
  "ip_addresses": [
    {
      "interface_id": "my-ns:10.0.0.1:0",
      "ip_address": "10.1.1.1",
      "prefixlen": 24
    }
  ],
  "collect_timestamp": 946684800
}
`)
	compactEvent := new(bytes.Buffer)
	err := json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")
}
