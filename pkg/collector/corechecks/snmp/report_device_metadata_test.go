package snmp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"
)

func Test_metricSender_reportNetworkDeviceMetadata_withoutInterfaces(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.TraceLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "debug")

	var storeWithoutIfName = &resultValueStore{
		columnValues: columnResultValuesType{},
	}

	sender := mocksender.NewMockSender("testID") // required to initiate aggregator
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	ms := &metricSender{
		sender: sender,
	}

	config := snmpConfig{
		ipAddress:    "1.2.3.4",
		deviceID:     "1234",
		deviceIDTags: []string{"device_name:127.0.0.1"},
		subnet:       "127.0.0.0/29",
	}
	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	assert.NoError(t, err)

	ms.reportNetworkDeviceMetadata(config, storeWithoutIfName, []string{"tag1", "tag2"}, collectTime)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "devices": [
        {
            "id": "1234",
            "id_tags": [
                "device_name:127.0.0.1"
            ],
            "name": "",
            "description": "",
            "ip_address": "1.2.3.4",
            "sys_object_id": "",
            "profile": "",
            "vendor": "",
            "subnet": "127.0.0.0/29",
            "tags": [
                "tag1",
                "tag2"
            ]
        }
    ],
	"collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.String(), "network-devices-metadata")

	w.Flush()
	logs := b.String()

	assert.Contains(t, logs, "Unable to build interfaces metadata: no interface indexes found")
}

func Test_metricSender_reportNetworkDeviceMetadata_withInterfaces(t *testing.T) {
	var storeWithIfName = &resultValueStore{
		columnValues: columnResultValuesType{
			"1.3.6.1.2.1.31.1.1.1.1": {
				"1": snmpValueType{value: float64(21)},
				"2": snmpValueType{value: float64(22)},
			},
		},
	}
	sender := mocksender.NewMockSender("testID") // required to initiate aggregator
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	ms := &metricSender{
		sender: sender,
	}

	config := snmpConfig{
		ipAddress:    "1.2.3.4",
		deviceID:     "1234",
		deviceIDTags: []string{"device_name:127.0.0.1"},
		subnet:       "127.0.0.0/29",
	}

	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	assert.NoError(t, err)
	ms.reportNetworkDeviceMetadata(config, storeWithIfName, []string{"tag1", "tag2"}, collectTime)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "devices": [
        {
            "id": "1234",
            "id_tags": [
                "device_name:127.0.0.1"
            ],
            "name": "",
            "description": "",
            "ip_address": "1.2.3.4",
            "sys_object_id": "",
            "profile": "",
            "vendor": "",
            "subnet": "127.0.0.0/29",
            "tags": [
                "tag1",
                "tag2"
            ]
        }
    ],
    "interfaces": [
        {
            "device_id": "1234",
            "index": 1,
            "name": "21",
            "alias": "",
            "description": "",
            "mac_address": "",
            "admin_status": 0,
            "oper_status": 0
        },
        {
            "device_id": "1234",
            "index": 2,
            "name": "22",
            "alias": "",
            "description": "",
            "mac_address": "",
            "admin_status": 0,
            "oper_status": 0
        }
    ],
	"collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.String(), "network-devices-metadata")
}

func Test_batchPayloads(t *testing.T) {
	collectTime := mockTimeNow()
	deviceID := "123"
	device := metadata.DeviceMetadata{ID: deviceID}

	var interfaces []metadata.InterfaceMetadata
	for i := 0; i < 350; i++ {
		interfaces = append(interfaces, metadata.InterfaceMetadata{DeviceID: deviceID, Index: int32(i)})
	}
	payloads := batchPayloads("127.0.0.0/30", collectTime, 100, device, interfaces)

	assert.Equal(t, 4, len(payloads))

	assert.Equal(t, "127.0.0.0/30", payloads[0].Subnet)
	assert.Equal(t, int64(946684800), payloads[0].CollectTimestamp)
	assert.Equal(t, []metadata.DeviceMetadata{device}, payloads[0].Devices)
	assert.Equal(t, 99, len(payloads[0].Interfaces))
	assert.Equal(t, interfaces[0:99], payloads[0].Interfaces)

	assert.Equal(t, "127.0.0.0/30", payloads[1].Subnet)
	assert.Equal(t, int64(946684800), payloads[1].CollectTimestamp)
	assert.Equal(t, 0, len(payloads[1].Devices))
	assert.Equal(t, 100, len(payloads[1].Interfaces))
	assert.Equal(t, interfaces[99:199], payloads[1].Interfaces)

	assert.Equal(t, 0, len(payloads[2].Devices))
	assert.Equal(t, 100, len(payloads[2].Interfaces))
	assert.Equal(t, interfaces[199:299], payloads[2].Interfaces)

	assert.Equal(t, 0, len(payloads[3].Devices))
	assert.Equal(t, 51, len(payloads[3].Interfaces))
	assert.Equal(t, interfaces[299:350], payloads[3].Interfaces)
}
