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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/profile"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func Test_metricSender_reportNetworkDeviceMetadata_withoutInterfaces(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.TraceLvl, "[%LEVEL] %FuncShort: %Msg")
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
		ProfileName:        "my-profile",
		ProfileProvider: profile.StaticProvider(profile.ProfileConfigMap{
			"my-profile": profile.ProfileConfig{
				Definition: profiledefinition.ProfileDefinition{
					Name:    "my-profile",
					Version: 10,
					Metadata: profiledefinition.MetadataConfig{
						"device": {
							Fields: map[string]profiledefinition.MetadataField{
								"name": {
									// Should use value from Symbol `1.3.6.1.2.1.1.5.0`
									Symbol: profiledefinition.SymbolConfig{
										OID:  "1.3.6.1.2.1.1.5.0",
										Name: "sysName",
									},
									Symbols: []profiledefinition.SymbolConfig{
										{
											OID:  "1.2.99",
											Name: "doesNotExist",
										},
									},
								},
								"description": {
									// Should use value from first element in Symbols `1.3.6.1.2.1.1.1.0`
									Symbol: profiledefinition.SymbolConfig{
										OID:  "1.9999",
										Name: "doesNotExist",
									},
									Symbols: []profiledefinition.SymbolConfig{
										{
											OID:  "1.3.6.1.2.1.1.1.0",
											Name: "sysDescr",
										},
									},
								},
								"location": {
									// Should use value from first element in Symbols `1.3.6.1.2.1.1.1.0`
									Symbol: profiledefinition.SymbolConfig{
										OID:  "1.9999",
										Name: "doesNotExist",
									},
									Symbols: []profiledefinition.SymbolConfig{
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
								"type": {
									Value: "router",
								},
							},
						},
					},
				},
			},
		}),
	}
	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	require.NoError(t, err)
	profile, err := config.BuildProfile("")
	require.NoError(t, err)

	ms.ReportNetworkDeviceMetadata(config, profile, storeWithoutIfName, []string{"tag1", "tag2"}, nil, collectTime, metadata.DeviceStatusReachable, metadata.DeviceStatusReachable, nil)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
	"integration": "snmp",
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
			"ping_status":1,
            "name": "my-sys-name",
            "description": "my-sys-descr",
            "location": "my-sys-location",
            "profile": "my-profile",
            "profile_version": 10,
            "subnet": "127.0.0.0/29",
			"integration": "snmp",
			"device_type": "router"
        }
    ],
    "collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")

	w.Flush()
	logs := b.String()

	assert.Contains(t, logs, "Unable to build interfaces metadata: no interface indexes found")
}

func Test_metricSender_reportNetworkDeviceMetadata_profileDeviceVendorFallback(t *testing.T) {
	profile.SetConfdPathAndCleanProfiles()

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

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig, nil)
	require.Nil(t, err)

	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	require.NoError(t, err)
	profile, err := config.BuildProfile("")
	require.NoError(t, err)

	ms.ReportNetworkDeviceMetadata(config, profile, storeWithoutIfName, []string{"tag1", "tag2"}, nil, collectTime,
		metadata.DeviceStatusReachable, metadata.DeviceStatusReachable, nil)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
	"integration": "snmp",
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
			"ping_status":1,
            "profile": "f5-big-ip",
            "vendor": "f5",
            "subnet": "127.0.0.0/29",
			"integration": "snmp",
			"device_type": "load_balancer"
        }
    ],
    "collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")
}

func Test_metricSender_reportNetworkDeviceMetadata_withDeviceInterfacesAndDiagnoses(t *testing.T) {
	var storeWithIfName = &valuestore.ResultValueStore{
		ColumnValues: valuestore.ColumnResultValuesType{
			"1.3.6.1.2.1.31.1.1.1.1": {
				"1": valuestore.ResultValue{Value: float64(21)},
				"2": valuestore.ResultValue{Value: float64(22)},
			},
			"1.3.6.1.2.1.2.2.1.7": {
				"1": valuestore.ResultValue{Value: float64(2)},
				"2": valuestore.ResultValue{Value: float64(2)},
			},
			"1.3.6.1.2.1.2.2.1.8": {
				"1": valuestore.ResultValue{Value: float64(1)},
				"2": valuestore.ResultValue{Value: float64(2)},
			},
			"1.3.6.1.2.1.31.1.1.1.18": {
				"1": valuestore.ResultValue{Value: "ifAlias1"},
				"2": valuestore.ResultValue{Value: ""},
			},
		},
	}
	sender := mocksender.NewMockSender("testID") // required to initiate aggregator
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	ms := &MetricSender{
		hostname: "test",
		sender:   sender,
		interfaceConfigs: []snmpintegration.InterfaceConfig{
			{
				MatchField: "index",
				MatchValue: "2",
				Tags: []string{
					"muted",
					"someKey:someValue",
				},
			},
		},
	}

	config := &checkconfig.CheckConfig{
		IPAddress:          "1.2.3.4",
		DeviceID:           "1234",
		DeviceIDTags:       []string{"device_name:127.0.0.1"},
		ResolvedSubnetName: "127.0.0.0/29",
		Namespace:          "my-ns",
	}
	profile := profiledefinition.ProfileDefinition{
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"type": {
						Value: "switch",
					},
				},
			},
			"interface": {
				Fields: map[string]profiledefinition.MetadataField{
					"name": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "ifName",
						},
					},
					"alias": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.31.1.1.1.18",
							Name: "ifAlias",
						},
					},
					"admin_status": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2.1.7",
							Name: "ifAdminStatus",
						},
					},
					"oper_status": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2.1.8",
							Name: "ifOperStatus",
						},
					},
				},
				IDTags: profiledefinition.MetricTagConfigList{
					profiledefinition.MetricTagConfig{
						Symbol: profiledefinition.SymbolConfigCompat{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "interface",
						},
						Tag: "interface",
					},
				},
			},
		},
	}

	diagnosis := []metadata.DiagnosisMetadata{{ResourceType: "device", ResourceID: "1234", Diagnoses: []metadata.Diagnosis{{
		Severity: "warn",
		Code:     "TEST_DIAGNOSIS",
		Message:  "Test",
	}}}}

	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	assert.NoError(t, err)
	ms.ReportNetworkDeviceMetadata(config, profile, storeWithIfName, []string{"tag1", "tag2"}, []string{"tag1", "tag2", "metric:tag"}, collectTime, metadata.DeviceStatusReachable, metadata.DeviceStatusUnreachable, diagnosis)

	ifTags1 := []string{"tag1", "tag2", "metric:tag", "status:down", "interface:21", "interface_alias:ifAlias1", "interface_index:1", "oper_status:up", "admin_status:down"}
	ifTags2 := []string{"tag1", "tag2", "metric:tag", "status:off", "interface:22", "interface_index:2", "oper_status:down", "admin_status:down", "muted", "someKey:someValue"}

	sender.AssertMetric(t, "Gauge", interfaceStatusMetric, 1., "test", ifTags1)
	sender.AssertMetric(t, "Gauge", interfaceStatusMetric, 1., "test", ifTags2)
	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
	"integration": "snmp",
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
			"ping_status":2,
            "subnet": "127.0.0.0/29",
			"integration": "snmp",
			"device_type": "switch"
        }
    ],
    "interfaces": [
        {
            "device_id": "1234",
            "id_tags": [
                "interface:21"
            ],
            "index": 1,
			"name": "21",
			"alias": "ifAlias1",
			"admin_status": 2,
			"oper_status": 1
        },
        {
            "device_id": "1234",
            "id_tags": [
                "interface:22"
            ],
            "index": 2,
            "name": "22",
			"admin_status": 2,
			"oper_status": 2
        }
    ],
    "diagnoses": [
      {
        "resource_type": "device",
        "resource_id": "1234",
        "diagnoses": [
          {
            "severity": "warn",
            "message": "Test",
            "code": "TEST_DIAGNOSIS"
          }
        ]
      }
    ],
    "collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")
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
	}
	profile := profiledefinition.ProfileDefinition{
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"name": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.999",
							Name: "doesNotExist",
						},
						Value: "my-fallback-value",
					},
					"type": {
						Value: "firewall",
					},
				},
			},
		},
	}
	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	assert.NoError(t, err)

	ms.ReportNetworkDeviceMetadata(config, profile, emptyMetadataStore, []string{"tag1", "tag2"}, nil, collectTime, metadata.DeviceStatusReachable, metadata.DeviceStatusUnreachable, nil)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
	"integration": "snmp",
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
			"ping_status":2,
            "name": "my-fallback-value",
            "subnet": "127.0.0.0/29",
			"integration": "snmp",
			"device_type": "firewall"
        }
    ],
    "collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")
}

func Test_metricSender_reportNetworkDeviceMetadata_pingCanConnect_Nil(t *testing.T) {
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
	}
	profile := profiledefinition.ProfileDefinition{
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"name": {
						Symbol: profiledefinition.SymbolConfig{
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

	ms.ReportNetworkDeviceMetadata(config, profile, emptyMetadataStore, []string{"tag1", "tag2"}, nil, collectTime, metadata.DeviceStatusReachable, 0, nil)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
	"integration": "snmp",
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
            "subnet": "127.0.0.0/29",
			"integration": "snmp",
			"device_type": "other"
        }
    ],
    "collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")
}

func Test_metricSender_reportNetworkDeviceMetadata_pingCanConnect_True(t *testing.T) {
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
	}
	profile := profiledefinition.ProfileDefinition{
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"name": {
						Symbol: profiledefinition.SymbolConfig{
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

	ms.ReportNetworkDeviceMetadata(config, profile, emptyMetadataStore, []string{"tag1", "tag2"}, nil, collectTime, metadata.DeviceStatusReachable, metadata.DeviceStatusUnreachable, nil)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
	"integration": "snmp",
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
			"ping_status":2,
            "name": "my-fallback-value",
            "subnet": "127.0.0.0/29",
			"integration": "snmp",
			"device_type": "other"
        }
    ],
    "collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")
}

func Test_metricSender_reportNetworkDeviceMetadata_pingCanConnect_False(t *testing.T) {
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
	}
	profile := profiledefinition.ProfileDefinition{
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"name": {
						Symbol: profiledefinition.SymbolConfig{
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

	ms.ReportNetworkDeviceMetadata(config, profile, emptyMetadataStore, []string{"tag1", "tag2"}, nil, collectTime, metadata.DeviceStatusReachable, metadata.DeviceStatusUnreachable, nil)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
	"integration": "snmp",
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
			"ping_status":2,
            "name": "my-fallback-value",
            "subnet": "127.0.0.0/29",
			"integration": "snmp",
			"device_type": "other"
        }
    ],
    "collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")
}

func Test_metricSender_reportNetworkDeviceMetadata_vpnTunnels(t *testing.T) {
	var store = &valuestore.ResultValueStore{
		ColumnValues: valuestore.ColumnResultValuesType{
			// Outside IPs
			"1.3.6.1.4.1.9.9.171.1.3.2.1.4": {
				"1": valuestore.ResultValue{
					Value: []byte{0x0A, 0x00, 0x00, 0x01}, // 10.0.0.1
				},
				"2": valuestore.ResultValue{
					Value: []byte{0x1E, 0x00, 0x00, 0x01}, // 30.0.0.1
				},
				"3": valuestore.ResultValue{
					Value: []byte{0x32, 0x00, 0x00, 0x01}, // 50.0.0.1
				},
			},
			"1.3.6.1.4.1.9.9.171.1.3.2.1.5": {
				"1": valuestore.ResultValue{
					Value: []byte{0x14, 0x00, 0x00, 0x01}, // 20.0.0.1
				},
				"2": valuestore.ResultValue{
					Value: []byte{0x28, 0x00, 0x00, 0x01}, // 40.0.0.1
				},
				"3": valuestore.ResultValue{
					Value: []byte{0x3C, 0x00, 0x00, 0x01}, // 60.0.0.1
				},
			},

			// Route Table (Current)
			"1.3.6.1.2.1.4.24.7.1.7": { // Interface Index
				"1.4.100.0.0.0.16.2.0.0.0.0": valuestore.ResultValue{
					Value: "2",
				},
				"1.4.110.0.0.0.24.2.0.0.1.4.40.0.0.1": valuestore.ResultValue{
					Value: "4",
				},
				"1.4.120.0.0.0.24.2.0.0.0.0": valuestore.ResultValue{
					Value: "6",
				},
			},
			"1.3.6.1.2.1.4.24.7.1.17": { // Status
				"1.4.100.0.0.0.16.2.0.0.0.0": valuestore.ResultValue{
					Value: "1",
				},
				"1.4.110.0.0.0.24.2.0.0.1.4.40.0.0.1": valuestore.ResultValue{
					Value: "1",
				},
				"1.4.120.0.0.0.24.2.0.0.0.0": valuestore.ResultValue{
					Value: "1",
				},
			},
			// Route Table (Deprecated)
			"1.3.6.1.2.1.4.24.4.1.5": { // Interface Index
				"100.1.0.0.255.255.0.0.0.0.0.0.0": valuestore.ResultValue{
					Value: "2",
				},
				"110.0.0.0.255.255.255.0.0.40.0.0.1": valuestore.ResultValue{
					Value: "4",
				},
				"110.1.0.0.255.255.255.0.0.40.0.0.1": valuestore.ResultValue{
					Value: "4",
				},
				"120.0.0.0.255.255.0.0.0.0.0.0.0": valuestore.ResultValue{
					Value: "6",
				},
			},
			"1.3.6.1.2.1.4.24.4.1.16": { // Status
				"100.1.0.0.255.255.0.0.0.0.0.0.0": valuestore.ResultValue{
					Value: "1",
				},
				"110.0.0.0.255.255.255.0.0.40.0.0.1": valuestore.ResultValue{
					Value: "1",
				},
				"110.1.0.0.255.255.255.0.0.40.0.0.1": valuestore.ResultValue{
					Value: "1",
				},
				"120.0.0.0.255.255.0.0.0.0.0.0.0": valuestore.ResultValue{
					Value: "1",
				},
			},

			// Tunnels (Current)
			"1.3.6.1.2.1.10.131.1.1.3.1.6": { // Interface Index
				"1.4.10.0.0.1.4.20.0.0.1.1.1": valuestore.ResultValue{
					Value: "2",
				},
				"1.4.50.0.0.1.4.60.0.0.1.1.2": valuestore.ResultValue{
					Value: "6",
				},
			},
			// Tunnels (Deprecated)
			"1.3.6.1.2.1.10.131.1.1.2.1.5": { // Interface Index
				"10.0.0.1.20.0.0.1.1.1": valuestore.ResultValue{
					Value: "2",
				},
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
		ResolvedSubnetName: "127.0.0.0/29",
		Namespace:          "my-ns",
	}
	layout := "2006-01-02 15:04:05"
	str := "2014-11-12 11:45:26"
	collectTime, err := time.Parse(layout, str)
	require.NoError(t, err)

	profile := profiledefinition.ProfileDefinition{
		Metadata: profiledefinition.MetadataConfig{
			"cisco_ipsec_tunnel": {
				Fields: map[string]profiledefinition.MetadataField{
					"local_outside_ip": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.4.1.9.9.171.1.3.2.1.4",
							Name: "cipSecTunLocalAddr",
						},
					},
					"remote_outside_ip": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.4.1.9.9.171.1.3.2.1.5",
							Name: "cipSecTunRemoteAddr",
						},
					},
				},
			},
			"ipforward_deprecated": {
				Fields: map[string]profiledefinition.MetadataField{
					"if_index": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.4.24.4.1.5",
							Name: "ipCidrRouteIfIndex",
						},
					},
					"route_status": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.4.24.4.1.16",
							Name: "ipCidrRouteStatus",
						},
					},
				},
			},
			"ipforward": {
				Fields: map[string]profiledefinition.MetadataField{
					"if_index": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.4.24.7.1.7",
							Name: "inetCidrRouteIfIndex",
						},
					},
					"route_status": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.4.24.7.1.17",
							Name: "inetCidrRouteStatus",
						},
					},
				},
			},
			"tunnel_config_deprecated": {
				Fields: map[string]profiledefinition.MetadataField{
					"if_index": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.10.131.1.1.2.1.5",
							Name: "tunnelConfigIfIndex",
						},
					},
				},
			},
			"tunnel_config": {
				Fields: map[string]profiledefinition.MetadataField{
					"if_index": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.10.131.1.1.3.1.6",
							Name: "tunnelInetConfigIfIndex",
						},
					},
				},
			},
		},
	}

	ms.ReportNetworkDeviceMetadata(config, profile, store, nil, nil, collectTime, metadata.DeviceStatusReachable, metadata.DeviceStatusReachable, nil)

	// language=json
	event := []byte(`
{
    "subnet": "127.0.0.0/29",
    "namespace": "my-ns",
    "integration": "snmp",
    "devices": [
        {
            "id": "1234",
            "id_tags": null,
            "tags": [],
            "ip_address": "1.2.3.4",
            "status": 1,
            "ping_status": 1,
            "subnet": "127.0.0.0/29",
            "integration": "snmp",
            "device_type": "other"
        }
    ],
    "vpn_tunnels": [
        {
            "device_id": "1234",
            "interface_id": "1234:2",
            "local_outside_ip": "10.0.0.1",
            "remote_outside_ip": "20.0.0.1",
            "protocol": "ipsec",
            "route_addresses": [
                "100.0.0.0/16",
                "100.1.0.0/16"
            ]
        },
        {
            "device_id": "1234",
            "local_outside_ip": "30.0.0.1",
            "remote_outside_ip": "40.0.0.1",
            "protocol": "ipsec",
            "route_addresses": [
                "110.0.0.0/24",
                "110.1.0.0/24"
            ]
        },
        {
            "device_id": "1234",
            "interface_id": "1234:6",
            "local_outside_ip": "50.0.0.1",
            "remote_outside_ip": "60.0.0.1",
            "protocol": "ipsec",
            "route_addresses": [
                "120.0.0.0/16",
                "120.0.0.0/24"
            ]
        }
    ],
    "collect_timestamp":1415792726
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")
}

func TestComputeInterfaceStatus(t *testing.T) {
	type testCase struct {
		ifAdminStatus metadata.IfAdminStatus
		ifOperStatus  metadata.IfOperStatus
		status        metadata.InterfaceStatus
	}

	// Test the method with only valid input for ifAdminStatus and ifOperStatus
	allTests := []testCase{
		// Valid test cases
		{metadata.AdminStatusUp, metadata.OperStatusUp, metadata.InterfaceStatusUp},
		{metadata.AdminStatusUp, metadata.OperStatusDown, metadata.InterfaceStatusDown},
		{metadata.AdminStatusUp, metadata.OperStatusTesting, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusUp, metadata.OperStatusUnknown, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusUp, metadata.OperStatusDormant, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusUp, metadata.OperStatusNotPresent, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusUp, metadata.OperStatusLowerLayerDown, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusDown, metadata.OperStatusUp, metadata.InterfaceStatusDown},
		{metadata.AdminStatusDown, metadata.OperStatusDown, metadata.InterfaceStatusOff},
		{metadata.AdminStatusDown, metadata.OperStatusTesting, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusDown, metadata.OperStatusUnknown, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusDown, metadata.OperStatusDormant, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusDown, metadata.OperStatusNotPresent, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusDown, metadata.OperStatusLowerLayerDown, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusTesting, metadata.OperStatusUp, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusTesting, metadata.OperStatusDown, metadata.InterfaceStatusDown},
		{metadata.AdminStatusTesting, metadata.OperStatusTesting, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusTesting, metadata.OperStatusUnknown, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusTesting, metadata.OperStatusDormant, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusTesting, metadata.OperStatusNotPresent, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusTesting, metadata.OperStatusLowerLayerDown, metadata.InterfaceStatusWarning},

		// Invalid ifOperStatus
		{metadata.AdminStatusUp, 0, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusUp, 8, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusUp, 100, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusDown, 0, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusDown, 8, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusDown, 100, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusTesting, 0, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusTesting, 8, metadata.InterfaceStatusWarning},
		{metadata.AdminStatusTesting, 100, metadata.InterfaceStatusWarning},

		// Invalid ifAdminStatus
		{0, metadata.OperStatusUnknown, metadata.InterfaceStatusDown},
		{0, metadata.OperStatusDown, metadata.InterfaceStatusDown},
		{0, metadata.OperStatusUp, metadata.InterfaceStatusDown},
		{4, metadata.OperStatusUp, metadata.InterfaceStatusDown},
		{4, metadata.OperStatusDown, metadata.InterfaceStatusDown},
		{4, metadata.OperStatusTesting, metadata.InterfaceStatusDown},
		{100, metadata.OperStatusUp, metadata.InterfaceStatusDown},
		{100, metadata.OperStatusDown, metadata.InterfaceStatusDown},
		{100, metadata.OperStatusTesting, metadata.InterfaceStatusDown},
	}
	for _, test := range allTests {
		assert.Equal(t, test.status, computeInterfaceStatus(test.ifAdminStatus, test.ifOperStatus))
	}
}

func Test_getRemManIPAddrByLLDPRemIndex(t *testing.T) {
	indexes := []string{
		// IPv4
		"0.102.2.1.4.10.250.0.7",
		"0.102.99.1.4.10.250.0.8",

		// IPv6
		"370.5.1.2.16.254.128.0.0.0.0.0.0.26.146.164.255.254.48.12.1",

		// Invalid
		"0.102.2.1.4.10.250", // too short, ignored
	}
	remManIPAddrByLLDPRemIndex := getRemManIPAddrByLLDPRemIndex(indexes)
	expectedResult := map[string]string{
		"2":  "10.250.0.7",
		"99": "10.250.0.8",
	}
	assert.Equal(t, expectedResult, remManIPAddrByLLDPRemIndex)
}

func Test_resolveLocalInterface(t *testing.T) {
	interfaceIndexByIDType := map[string]map[string][]int32{
		"mac_address": {
			"00:00:00:00:00:01": []int32{1},
			"00:00:00:00:00:02": []int32{2},
			"00:00:00:00:00:03": []int32{3, 4},
		},
		"interface_name": {
			"eth1": []int32{1},
			"eth2": []int32{2},
			"eth3": []int32{3}, // eth3 is both a name and alias, and reference the same interface
			"eth4": []int32{4}, // eth4 is both a name and alias, and reference two different interfaces
		},
		"interface_alias": {
			"alias1": []int32{1},
			"alias2": []int32{2},
			"eth3":   []int32{3},
			"eth4":   []int32{44},
		},
		"interface_index": {
			"1": []int32{1},
			"2": []int32{2},
		},
	}
	deviceID := "default:1.2.3.4"

	tests := []struct {
		name        string
		localIDType string
		localID     string
		expectedID  string
	}{
		{
			name:        "mac_address",
			localIDType: "mac_address",
			localID:     "00:00:00:00:00:01",
			expectedID:  "default:1.2.3.4:1",
		},
		{
			name:        "mac_address cannot resolve due to multiple results",
			localIDType: "mac_address",
			localID:     "00:00:00:00:00:03",
			expectedID:  "",
		},
		{
			name:        "interface_name",
			localIDType: "interface_name",
			localID:     "eth2",
			expectedID:  "default:1.2.3.4:2",
		},
		{
			name:        "interface_alias",
			localIDType: "interface_alias",
			localID:     "alias2",
			expectedID:  "default:1.2.3.4:2",
		},
		{
			name:        "mac_address by trying",
			localIDType: "",
			localID:     "00:00:00:00:00:01",
			expectedID:  "default:1.2.3.4:1",
		},
		{
			name:        "interface_name by trying",
			localIDType: "",
			localID:     "eth2",
			expectedID:  "default:1.2.3.4:2",
		},
		{
			name:        "interface_alias by trying",
			localIDType: "",
			localID:     "alias2",
			expectedID:  "default:1.2.3.4:2",
		},
		{
			name:        "interface_alias+interface_name match with same interface should resolve",
			localIDType: "",
			localID:     "eth3",
			expectedID:  "default:1.2.3.4:3",
		},
		{
			name:        "interface_alias+interface_name match with different interface should not resolve",
			localIDType: "",
			localID:     "eth4",
			expectedID:  "",
		},
		{
			name:        "interface_index by trying",
			localIDType: "",
			localID:     "2",
			expectedID:  "default:1.2.3.4:2",
		},
		{
			name:        "mac_address not found",
			localIDType: "mac_address",
			localID:     "00:00:00:00:00:99",
			expectedID:  "",
		},
		{
			name:        "invalid",
			localIDType: "invalid_type",
			localID:     "invalidID",
			expectedID:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedID, resolveLocalInterface(deviceID, interfaceIndexByIDType, tt.localIDType, tt.localID))
		})
	}
}

func Test_buildInterfaceIndexByIDType(t *testing.T) {
	// Arrange
	interfaces := []metadata.InterfaceMetadata{
		{
			DeviceID:   "default:1.2.3.4",
			Index:      1,
			MacAddress: "00:00:00:00:00:01",
			Name:       "eth1",
			Alias:      "alias1",
		},
		{
			DeviceID:   "default:1.2.3.4",
			Index:      2,
			MacAddress: "00:00:00:00:00:02",
			Name:       "eth2",
			Alias:      "alias2",
		},
		{
			DeviceID:   "default:1.2.3.4",
			Index:      3,
			MacAddress: "00:00:00:00:00:02",
			Name:       "eth3",
			Alias:      "alias3",
		},
	}

	// Act
	interfaceIndexByIDType := buildInterfaceIndexByIDType(interfaces)

	// Assert
	expectedInterfaceIndexByIDType := map[string]map[string][]int32{
		"mac_address": {
			"00:00:00:00:00:01": []int32{1},
			"00:00:00:00:00:02": []int32{2, 3},
		},
		"interface_name": {
			"eth1": []int32{1},
			"eth2": []int32{2},
			"eth3": []int32{3},
		},
		"interface_alias": {
			"alias1": []int32{1},
			"alias2": []int32{2},
			"alias3": []int32{3},
		},
		"interface_index": {
			"1": []int32{1},
			"2": []int32{2},
			"3": []int32{3},
		},
	}
	assert.Equal(t, expectedInterfaceIndexByIDType, interfaceIndexByIDType)
}
