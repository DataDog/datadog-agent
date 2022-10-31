// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func Test_metricSender_reportNetworkDeviceMetadata_withoutInterfaces(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.TraceLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "debug")

	var storeWithoutIfName = &valuestore.ResultValueStore{
		ScalarValues: valuestore.ScalarResultValuesType{
			"1.3.6.1.2.1.1.5.0": valuestore.ResultValue{
				Value: "my-sys-name",
			},
			"1.3.6.1.2.1.1.1.0": valuestore.ResultValue{
				Value: "my-sys-descr",
			},
			"1.3.6.1.2.1.1.6.0": valuestore.ResultValue{
				Value: "my-sys-location",
			},
		},
		ColumnValues: valuestore.ColumnResultValuesType{},
	}

	sender := mocksender.NewMockSender("testID") // required to initiate aggregator
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	ms := &MetricSender{
		sender: sender,
	}

	config := &checkconfig.CheckConfig{
		IPAddress:          "1.2.3.4",
		DeviceID:           "1234",
		DeviceIDTags:       []string{"device_name:127.0.0.1"},
		ResolvedSubnetName: "127.0.0.0/29",
		Namespace:          "my-ns",
		Metadata: checkconfig.MetadataConfig{
			"device": {
				Fields: map[string]checkconfig.MetadataField{
					"name": {
						// Should use value from Symbol `1.3.6.1.2.1.1.5.0`
						Symbol: checkconfig.SymbolConfig{
							OID:  "1.3.6.1.2.1.1.5.0",
							Name: "sysName",
						},
						Symbols: []checkconfig.SymbolConfig{
							{
								OID:  "1.2.99",
								Name: "doesNotExist",
							},
						},
					},
					"description": {
						// Should use value from first element in Symbols `1.3.6.1.2.1.1.1.0`
						Symbol: checkconfig.SymbolConfig{
							OID:  "1.9999",
							Name: "doesNotExist",
						},
						Symbols: []checkconfig.SymbolConfig{
							{
								OID:  "1.3.6.1.2.1.1.1.0",
								Name: "sysDescr",
							},
						},
					},
					"location": {
						// Should use value from first element in Symbols `1.3.6.1.2.1.1.1.0`
						Symbol: checkconfig.SymbolConfig{
							OID:  "1.9999",
							Name: "doesNotExist",
						},
						Symbols: []checkconfig.SymbolConfig{
							{
								OID:  "1.888",
								Name: "doesNotExist2",
							},
							{
								OID:  "1.3.6.1.2.1.1.6.0",
								Name: "sysLocation",
							},
							{
								OID:  "1.7777",
								Name: "doesNotExist2",
							},
						},
					},
				},
			},
		},
	}
	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	assert.NoError(t, err)

	ms.ReportNetworkDeviceMetadata(config, storeWithoutIfName, []string{"tag1", "tag2"}, collectTime, metadata.DeviceStatusReachable)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
    "devices": [
        {
            "id": "1234",
            "id_tags": [
                "device_name:127.0.0.1"
            ],
            "tags": [
                "tag1",
                "tag2"
            ],
            "ip_address": "1.2.3.4",
            "status":1,
            "name": "my-sys-name",
            "description": "my-sys-descr",
            "location": "my-sys-location",
            "subnet": "127.0.0.0/29"
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

func Test_metricSender_reportNetworkDeviceMetadata_profileDeviceVendorFallback(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()

	var storeWithoutIfName = &valuestore.ResultValueStore{
		ColumnValues: valuestore.ColumnResultValuesType{},
	}

	sender := mocksender.NewMockSender("testID") // required to initiate aggregator
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	ms := &MetricSender{
		sender: sender,
	}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
namespace: my-ns
profile: f5-big-ip
tags:
  - 'autodiscovery_subnet:127.0.0.0/29'
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
 f5-big-ip:
   definition_file: f5-big-ip.yaml
`)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	assert.NoError(t, err)

	ms.ReportNetworkDeviceMetadata(config, storeWithoutIfName, []string{"tag1", "tag2"}, collectTime, metadata.DeviceStatusReachable)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
    "devices": [
        {
            "id": "my-ns:1.2.3.4",
            "id_tags": [
                "device_namespace:my-ns",
                "snmp_device:1.2.3.4"
            ],
            "tags": [
                "tag1",
                "tag2"
            ],
            "ip_address": "1.2.3.4",
            "status":1,
            "profile": "f5-big-ip",
            "vendor": "f5",
            "subnet": "127.0.0.0/29"
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

func Test_metricSender_reportNetworkDeviceMetadata_withInterfaces(t *testing.T) {
	var storeWithIfName = &valuestore.ResultValueStore{
		ColumnValues: valuestore.ColumnResultValuesType{
			"1.3.6.1.2.1.31.1.1.1.1": {
				"1": valuestore.ResultValue{Value: float64(21)},
				"2": valuestore.ResultValue{Value: float64(22)},
			},
		},
	}
	sender := mocksender.NewMockSender("testID") // required to initiate aggregator
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	ms := &MetricSender{
		sender: sender,
	}

	config := &checkconfig.CheckConfig{
		IPAddress:          "1.2.3.4",
		DeviceID:           "1234",
		DeviceIDTags:       []string{"device_name:127.0.0.1"},
		ResolvedSubnetName: "127.0.0.0/29",
		Namespace:          "my-ns",
		Metadata: checkconfig.MetadataConfig{
			"interface": {
				Fields: map[string]checkconfig.MetadataField{
					"name": {
						Symbol: checkconfig.SymbolConfig{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "ifName",
						},
					},
				},
				IDTags: checkconfig.MetricTagConfigList{
					checkconfig.MetricTagConfig{
						Column: checkconfig.SymbolConfig{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "interface",
						},
						Tag: "interface",
					},
				},
			},
		},
	}

	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	assert.NoError(t, err)
	ms.ReportNetworkDeviceMetadata(config, storeWithIfName, []string{"tag1", "tag2"}, collectTime, metadata.DeviceStatusReachable)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
    "devices": [
        {
            "id": "1234",
            "id_tags": [
                "device_name:127.0.0.1"
            ],
            "tags": [
                "tag1",
                "tag2"
            ],
            "ip_address": "1.2.3.4",
            "status":1,
            "subnet": "127.0.0.0/29"
        }
    ],
    "interfaces": [
        {
            "device_id": "1234",
            "id_tags": [
                "interface:21"
            ],
            "index": 1,
            "name": "21"
        },
        {
            "device_id": "1234",
            "id_tags": [
                "interface:22"
            ],
            "index": 2,
            "name": "22"
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

func Test_metricSender_reportNetworkDeviceMetadata_fallbackOnFieldValue(t *testing.T) {
	var emptyMetadataStore = &valuestore.ResultValueStore{
		ColumnValues: valuestore.ColumnResultValuesType{},
	}

	sender := mocksender.NewMockSender("testID") // required to initiate aggregator
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	ms := &MetricSender{
		sender: sender,
	}

	config := &checkconfig.CheckConfig{
		IPAddress:          "1.2.3.4",
		DeviceID:           "1234",
		DeviceIDTags:       []string{"device_name:127.0.0.1"},
		ResolvedSubnetName: "127.0.0.0/29",
		Namespace:          "my-ns",
		Metadata: checkconfig.MetadataConfig{
			"device": {
				Fields: map[string]checkconfig.MetadataField{
					"name": {
						Symbol: checkconfig.SymbolConfig{
							OID:  "1.999",
							Name: "doesNotExist",
						},
						Value: "my-fallback-value",
					},
				},
			},
		},
	}
	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	assert.NoError(t, err)

	ms.ReportNetworkDeviceMetadata(config, emptyMetadataStore, []string{"tag1", "tag2"}, collectTime, metadata.DeviceStatusReachable)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
    "devices": [
        {
            "id": "1234",
            "id_tags": [
                "device_name:127.0.0.1"
            ],
            "tags": [
                "tag1",
                "tag2"
            ],
            "ip_address": "1.2.3.4",
            "status":1,
            "name": "my-fallback-value",
            "subnet": "127.0.0.0/29"
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
	collectTime := common.MockTimeNow()
	deviceID := "123"
	device := metadata.DeviceMetadata{ID: deviceID}

	var interfaces []metadata.InterfaceMetadata
	for i := 0; i < 350; i++ {
		interfaces = append(interfaces, metadata.InterfaceMetadata{DeviceID: deviceID, Index: int32(i)})
	}
	payloads := batchPayloads("my-ns", "127.0.0.0/30", collectTime, 100, device, interfaces, nil)

	assert.Equal(t, 4, len(payloads))

	assert.Equal(t, "my-ns", payloads[0].Namespace)
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
