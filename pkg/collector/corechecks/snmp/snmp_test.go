// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package snmp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type mockSession struct {
	mock.Mock
	connectErr error
	closeErr   error
	version    gosnmp.SnmpVersion
}

func (s *mockSession) Configure(config snmpConfig) error {
	return nil
}

func (s *mockSession) Connect() error {
	return s.connectErr
}

func (s *mockSession) Close() error {
	return s.closeErr
}

func (s *mockSession) Get(oids []string) (result *gosnmp.SnmpPacket, err error) {
	args := s.Mock.Called(oids)
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

func (s *mockSession) GetBulk(oids []string) (result *gosnmp.SnmpPacket, err error) {
	args := s.Mock.Called(oids)
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

func (s *mockSession) GetNext(oids []string) (result *gosnmp.SnmpPacket, err error) {
	args := s.Mock.Called(oids)
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

func (s *mockSession) GetVersion() gosnmp.SnmpVersion {
	return s.version
}

func createMockSession() *mockSession {
	session := &mockSession{}
	session.version = gosnmp.Version2c
	return session
}

func setConfdPathAndCleanProfiles() {
	globalProfileConfigMap = nil // make sure from the new confd path will be reloaded
	file, _ := filepath.Abs(filepath.Join(".", "test", "conf.d"))
	config.Datadog.Set("confd_path", file)
}

func TestBasicSample(t *testing.T) {
	setConfdPathAndCleanProfiles()
	session := createMockSession()
	check := Check{session: session}
	aggregator.InitAggregatorWithFlushInterval(nil, nil, "", 1*time.Hour)

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: ifNumber
  metric_tags:
  - symboltag1:1
  - symboltag2:2
- symbol:
    OID: 1.2.3.4.0
    name: aMetricWithExtractValue
    extract_value: '(\d+)C'
- table:
    OID: 1.3.6.1.2.1.2.2
    name: ifTable
  symbols:
  - OID: 1.3.6.1.2.1.2.2.1.14
    name: ifInErrors
  - OID: 1.3.6.1.2.1.2.2.1.20
    name: ifOutErrors

  metric_tags:
  - tag: if_index
    index: 1
  - tag: if_desc
    column:
      OID: 1.3.6.1.2.1.2.2.1.2
      name: ifDescr
metric_tags:
  - OID: 1.3.6.1.2.1.1.5.0
    symbol: sysName
    tag: snmp_host
tags:
  - "mytag:foo"
`)

	err := check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
			{
				Name:  "1.3.6.1.2.1.1.5.0",
				Type:  gosnmp.OctetString,
				Value: []byte("foo_sys_name"),
			},
			{
				Name:  "1.3.6.1.2.1.2.1",
				Type:  gosnmp.Integer,
				Value: 30,
			},
			{
				Name:  "1.2.3.4.0",
				Type:  gosnmp.OctetString,
				Value: []byte("22C"),
			},
		},
	}

	bulkPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.2.2.1.14.1",
				Type:  gosnmp.Integer,
				Value: 141,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.2.1",
				Type:  gosnmp.OctetString,
				Value: []byte("desc1"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.20.1",
				Type:  gosnmp.Integer,
				Value: 201,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.14.2",
				Type:  gosnmp.Integer,
				Value: 142,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.2.2",
				Type:  gosnmp.OctetString,
				Value: []byte("desc2"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.20.2",
				Type:  gosnmp.Integer,
				Value: 202,
			},
		},
	}

	bulkPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.2.2.1.15.1",
				Type:  gosnmp.Integer,
				Value: 141,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.3.2",
				Type:  gosnmp.OctetString,
				Value: []byte("none"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.21.1",
				Type:  gosnmp.Integer,
				Value: 201,
			},
		},
	}

	session.On("Get", mock.Anything).Return(&packet, nil)
	session.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.14", "1.3.6.1.2.1.2.2.1.2", "1.3.6.1.2.1.2.2.1.20"}).Return(&bulkPacket, nil)
	session.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.14.2", "1.3.6.1.2.1.2.2.1.2.2", "1.3.6.1.2.1.2.2.1.20.2"}).Return(&bulkPacket2, nil)

	err = check.Run()
	assert.Nil(t, err)

	snmpTags := []string{"snmp_device:1.2.3.4"}
	snmpGlobalTags := append(copyStrings(snmpTags), "snmp_host:foo_sys_name")
	snmpGlobalTagsWithLoader := append(copyStrings(snmpGlobalTags), "loader:core")
	row1Tags := append(copyStrings(snmpGlobalTags), "if_index:1", "if_desc:desc1")
	row2Tags := append(copyStrings(snmpGlobalTags), "if_index:2", "if_desc:desc2")
	scalarTags := append(copyStrings(snmpGlobalTags), "symboltag1:1", "symboltag2:2")

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", snmpGlobalTags)
	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpGlobalTags)
	sender.AssertMetric(t, "Gauge", "snmp.ifNumber", float64(30), "", scalarTags)
	sender.AssertMetric(t, "Gauge", "snmp.aMetricWithExtractValue", float64(22), "", snmpGlobalTags)
	sender.AssertMetric(t, "Gauge", "snmp.ifInErrors", float64(141), "", row1Tags)
	sender.AssertMetric(t, "Gauge", "snmp.ifInErrors", float64(142), "", row2Tags)
	sender.AssertMetric(t, "Gauge", "snmp.ifOutErrors", float64(201), "", row1Tags)
	sender.AssertMetric(t, "Gauge", "snmp.ifOutErrors", float64(202), "", row2Tags)

	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpGlobalTagsWithLoader)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpGlobalTagsWithLoader)
	sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 7, "", snmpGlobalTagsWithLoader)
}

func TestSupportedMetricTypes(t *testing.T) {
	setConfdPathAndCleanProfiles()
	session := createMockSession()
	check := Check{session: session}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
metrics:
- symbol:
    OID: 1.2.3.4.5.0
    name: SomeGaugeMetric
- symbol:
    OID: 1.2.3.4.5.1
    name: SomeCounter32Metric
- symbol:
    OID: 1.2.3.4.5.2
    name: SomeCounter64Metric
`)

	err := check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
			{
				Name:  "1.2.3.4.5.0",
				Type:  gosnmp.Integer,
				Value: 30,
			},
			{
				Name:  "1.2.3.4.5.1",
				Type:  gosnmp.Counter32,
				Value: 40,
			},
			{
				Name:  "1.2.3.4.5.2",
				Type:  gosnmp.Counter64,
				Value: 50,
			},
		},
	}

	session.On("Get", mock.Anything).Return(&packet, nil)

	err = check.Run()
	assert.Nil(t, err)

	tags := []string{"snmp_device:1.2.3.4"}
	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", tags)
	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", tags)
	sender.AssertMetric(t, "Gauge", "snmp.SomeGaugeMetric", float64(30), "", tags)
	sender.AssertMetric(t, "Rate", "snmp.SomeCounter32Metric", float64(40), "", tags)
	sender.AssertMetric(t, "Rate", "snmp.SomeCounter64Metric", float64(50), "", tags)
}

func TestProfile(t *testing.T) {
	timeNow = mockTimeNow
	aggregator.InitAggregatorWithFlushInterval(nil, nil, "", 1*time.Hour)
	setConfdPathAndCleanProfiles()

	session := createMockSession()
	check := Check{session: session}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
profile: f5-big-ip
collect_device_metadata: true
tags:
  - "mytag:val1"
  - "mytag:val1" # add duplicate tag for testing deduplication
  - "autodiscovery_subnet:127.0.0.0/30"
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
  f5-big-ip:
    definition_file: f5-big-ip.yaml
`)

	err := check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.5.0",
				Type:  gosnmp.OctetString,
				Value: []byte("foo_sys_name"),
			},
			{
				Name:  "1.3.6.1.2.1.1.1.0",
				Type:  gosnmp.OctetString,
				Value: []byte("my_desc"),
			},
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.2.3.4",
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
				Type:  gosnmp.Integer,
				Value: 30,
			},
			{
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
		},
	}

	bulkPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.2.2.1.13.1",
				Type:  gosnmp.Integer,
				Value: 131,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.14.1",
				Type:  gosnmp.Integer,
				Value: 141,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.2.1",
				Type:  gosnmp.OctetString,
				Value: []byte("ifDescRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.1",
				Type:  gosnmp.OctetString,
				Value: []byte("00:00:00:00:00:01"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.7.1",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.8.1",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
				Type:  gosnmp.OctetString,
				Value: []byte("nameRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.18.1",
				Type:  gosnmp.OctetString,
				Value: []byte("descRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.13.2",
				Type:  gosnmp.Integer,
				Value: 132,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.14.2",
				Type:  gosnmp.Integer,
				Value: 142,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.2.2",
				Type:  gosnmp.OctetString,
				Value: []byte("ifDescRow2"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.2",
				Type:  gosnmp.OctetString,
				Value: []byte("00:00:00:00:00:02"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.7.2",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.8.2",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.1.2",
				Type:  gosnmp.OctetString,
				Value: []byte("nameRow2"),
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.18.2",
				Type:  gosnmp.OctetString,
				Value: []byte("descRow2"),
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
		},
	}

	session.On("Get", []string{
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.999",
		"1.2.3.4.5",
		"1.3.6.1.2.1.1.5.0",
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
	}).Return(&packet, nil)
	session.On("GetBulk", []string{
		"1.3.6.1.2.1.2.2.1.13",
		"1.3.6.1.2.1.2.2.1.14",
		"1.3.6.1.2.1.2.2.1.2",
		"1.3.6.1.2.1.2.2.1.6",
		"1.3.6.1.2.1.2.2.1.7",
		"1.3.6.1.2.1.2.2.1.8",
		"1.3.6.1.2.1.31.1.1.1.1",
		"1.3.6.1.2.1.31.1.1.1.18",
	}).Return(&bulkPacket, nil)

	err = check.Run()
	assert.Nil(t, err)

	snmpTags := []string{"snmp_device:1.2.3.4", "snmp_profile:f5-big-ip", "device_vendor:f5", "snmp_host:foo_sys_name"}
	row1Tags := append(copyStrings(snmpTags), "interface:nameRow1", "interface_alias:descRow1")
	row2Tags := append(copyStrings(snmpTags), "interface:nameRow2", "interface_alias:descRow2")

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", snmpTags)
	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpTags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(141), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(142), "", row2Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(131), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(132), "", row2Tags)
	sender.AssertMetric(t, "Gauge", "snmp.sysStatMemoryTotal", float64(30), "", snmpTags)

	// language=json
	event := []byte(`
{
  "subnet": "127.0.0.0/30",
  "devices": [
    {
      "id": "173b2077d0770b8",
      "id_tags": [
        "mytag:val1",
        "snmp_device:1.2.3.4"
      ],
      "name": "foo_sys_name",
      "description": "my_desc",
      "ip_address": "1.2.3.4",
      "sys_object_id": "1.2.3.4",
      "profile": "f5-big-ip",
      "vendor": "f5",
      "subnet": "127.0.0.0/30",
      "tags": [
        "autodiscovery_subnet:127.0.0.0/30",
        "device_vendor:f5",
        "mytag:val1",
        "prefix:f",
        "snmp_device:1.2.3.4",
        "snmp_host:foo_sys_name",
        "snmp_profile:f5-big-ip",
        "some_tag:some_tag_value",
        "suffix:oo_sys_name"
      ]
    }
  ],
  "interfaces": [
    {
      "device_id": "173b2077d0770b8",
      "index": 1,
      "name": "nameRow1",
      "alias": "descRow1",
      "description": "ifDescRow1",
      "mac_address": "00:00:00:00:00:01",
      "admin_status": 1,
      "oper_status": 1
    },
    {
      "device_id": "173b2077d0770b8",
      "index": 2,
      "name": "nameRow2",
      "alias": "descRow2",
      "description": "ifDescRow2",
      "mac_address": "00:00:00:00:00:02",
      "admin_status": 1,
      "oper_status": 1
    }
  ],
  "collect_timestamp":946684800
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.String(), "network-devices-metadata")

	sender.AssertServiceCheck(t, "snmp.can_check", metrics.ServiceCheckOK, "", snmpTags, "")
}

func TestProfileWithSysObjectIdDetection(t *testing.T) {
	setConfdPathAndCleanProfiles()
	session := createMockSession()
	check := Check{session: session}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
  f5-big-ip:
    definition_file: f5-big-ip.yaml
`)

	err := check.Configure(rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	sysObjectIDPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
			{
				Name:  "1.3.6.1.2.1.1.5.0",
				Type:  gosnmp.OctetString,
				Value: []byte("foo_sys_name"),
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
				Type:  gosnmp.Integer,
				Value: 30,
			},
		},
	}

	bulkPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.2.2.1.13.1",
				Type:  gosnmp.Integer,
				Value: 131,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.14.1",
				Type:  gosnmp.Integer,
				Value: 141,
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
				Type:  gosnmp.OctetString,
				Value: []byte("nameRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.18.1",
				Type:  gosnmp.OctetString,
				Value: []byte("descRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.13.2",
				Type:  gosnmp.Integer,
				Value: 132,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.14.2",
				Type:  gosnmp.Integer,
				Value: 142,
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.1.2",
				Type:  gosnmp.OctetString,
				Value: []byte("nameRow2"),
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.18.2",
				Type:  gosnmp.OctetString,
				Value: []byte("descRow2"),
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
		},
	}

	session.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&sysObjectIDPacket, nil)
	session.On("Get", []string{"1.3.6.1.2.1.1.3.0", "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", "1.3.6.1.4.1.3375.2.1.1.2.1.44.999", "1.2.3.4.5", "1.3.6.1.2.1.1.5.0"}).Return(&packet, nil)
	session.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.13", "1.3.6.1.2.1.2.2.1.14", "1.3.6.1.2.1.31.1.1.1.1", "1.3.6.1.2.1.31.1.1.1.18"}).Return(&bulkPacket, nil)

	err = check.Run()
	assert.Nil(t, err)

	snmpTags := []string{"snmp_device:1.2.3.4", "snmp_profile:f5-big-ip", "device_vendor:f5", "snmp_host:foo_sys_name",
		"some_tag:some_tag_value", "prefix:f", "suffix:oo_sys_name"}
	row1Tags := append(copyStrings(snmpTags), "interface:nameRow1", "interface_alias:descRow1")
	row2Tags := append(copyStrings(snmpTags), "interface:nameRow2", "interface_alias:descRow2")

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", snmpTags)
	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpTags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(141), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(142), "", row2Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(131), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(132), "", row2Tags)
	sender.AssertMetric(t, "Gauge", "snmp.sysStatMemoryTotal", float64(30), "", snmpTags)

	assert.Equal(t, false, check.config.autodetectProfile)

	// Make sure we don't auto detect and add metrics twice if we already did that previously
	firstRunMetrics := check.config.metrics
	firstRunMetricsTags := check.config.metricTags
	err = check.Run()
	assert.Nil(t, err)

	assert.Len(t, check.config.metrics, len(firstRunMetrics))
	assert.Len(t, check.config.metricTags, len(firstRunMetricsTags))
}

func TestServiceCheckFailures(t *testing.T) {
	setConfdPathAndCleanProfiles()
	session := createMockSession()
	session.connectErr = fmt.Errorf("can't connect")
	check := Check{session: session}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
`)

	err := check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	err = check.Run()
	assert.Error(t, err, "snmp connection error: can't connect")

	snmpTags := []string{"snmp_device:1.2.3.4"}

	sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 0.0, "", snmpTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpTags)
	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpTags)
	sender.AssertServiceCheck(t, "snmp.can_check", metrics.ServiceCheckCritical, "", snmpTags, "snmp connection error: can't connect")
}

func TestCheckID(t *testing.T) {
	setConfdPathAndCleanProfiles()
	check1 := snmpFactory()
	check2 := snmpFactory()
	// language=yaml
	rawInstanceConfig1 := []byte(`
ip_address: 1.1.1.1
community_string: abc
`)
	// language=yaml
	rawInstanceConfig2 := []byte(`
ip_address: 2.2.2.2
community_string: abc
`)

	err := check1.Configure(rawInstanceConfig1, []byte(``), "test")
	assert.Nil(t, err)

	err = check2.Configure(rawInstanceConfig2, []byte(``), "test")
	assert.Nil(t, err)

	assert.Equal(t, check.ID("snmp:ed97702503abb6ec"), check1.ID())
	assert.Equal(t, check.ID("snmp:e4bdb13416d918f4"), check2.ID())
	assert.NotEqual(t, check1.ID(), check2.ID())
}

func TestCheck_Run(t *testing.T) {
	sysObjectIDPacketInvalidSysObjectIDMock := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.999999",
			},
		},
	}

	sysObjectIDPacketInvalidValueMock := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: 1.0,
			},
		},
	}

	sysObjectIDPacketInvalidConversionMock := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: gosnmp.SnmpPDU{},
			},
		},
	}

	sysObjectIDPacketOkMock := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}

	valuesPacketErrMock := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
			{
				Name:  "1.3.6.1.2.1.1.5.0",
				Type:  gosnmp.OctetString,
				Value: []byte("foo_sys_name"),
			},
			{
				Name:  "1.3.6.1.2.1.1.5.0",
				Type:  gosnmp.OctetString,
				Value: []byte("foo_sys_name"),
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
				Type:  gosnmp.Integer,
				Value: 30,
			},
		},
	}

	tests := []struct {
		name              string
		disableAggregator bool
		sessionConnError  error
		sysObjectIDPacket gosnmp.SnmpPacket
		sysObjectIDError  error
		valuesPacket      gosnmp.SnmpPacket
		valuesError       error
		expectedErr       string
	}{
		{
			name:             "connection error",
			sessionConnError: fmt.Errorf("can't connect"),
			expectedErr:      "snmp connection error: can't connect",
		},
		{
			name:             "failed to fetching sysobjectid",
			sysObjectIDError: fmt.Errorf("no sysobjectid"),
			expectedErr:      "failed to fetching sysobjectid: cannot get sysobjectid: no sysobjectid",
		},
		{
			name:        "unexpected values count",
			expectedErr: "failed to fetching sysobjectid: expected 1 value, but got 0: variables=[]",
		},
		{
			name:              "failed to fetching sysobjectid with invalid value",
			sysObjectIDPacket: sysObjectIDPacketInvalidValueMock,
			expectedErr:       "failed to fetching sysobjectid: error getting value from pdu: oid 1.3.6.1.2.1.1.2.0: ObjectIdentifier should be string type but got float64 type: gosnmp.SnmpPDU{Name:\"1.3.6.1.2.1.1.2.0\", Type:0x6, Value:1}",
		},
		{
			name:              "failed to fetching sysobjectid with conversion error",
			sysObjectIDPacket: sysObjectIDPacketInvalidConversionMock,
			expectedErr:       "failed to fetching sysobjectid: error getting value from pdu: oid 1.3.6.1.2.1.1.2.0: ObjectIdentifier should be string type but got gosnmp.SnmpPDU type: gosnmp.SnmpPDU{Name:\"1.3.6.1.2.1.1.2.0\", Type:0x6, Value:gosnmp.SnmpPDU{Name:\"\", Type:0x0, Value:interface {}(nil)}}",
		},
		{
			name:              "failed to get profile sys object id",
			sysObjectIDPacket: sysObjectIDPacketInvalidSysObjectIDMock,
			expectedErr:       "failed to get profile sys object id for `1.999999`: failed to get most specific profile for sysObjectID `1.999999`, for matched oids []: cannot get most specific oid from empty list of oids",
		},
		{
			name:              "failed to fetch values",
			sysObjectIDPacket: sysObjectIDPacketOkMock,
			valuesPacket:      valuesPacketErrMock,
			valuesError:       fmt.Errorf("no value"),
			expectedErr:       "failed to fetch values: failed to fetch scalar oids with batching: failed to fetch scalar oids: fetch scalar: error getting oids `[1.3.6.1.2.1.1.3.0 1.3.6.1.4.1.3375.2.1.1.2.1.44.0 1.3.6.1.4.1.3375.2.1.1.2.1.44.999 1.2.3.4.5 1.3.6.1.2.1.1.5.0]`: no value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setConfdPathAndCleanProfiles()
			session := createMockSession()
			session.connectErr = tt.sessionConnError
			check := Check{session: session}

			// language=yaml
			rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
`)

			err := check.Configure(rawInstanceConfig, []byte(``), "test")
			assert.Nil(t, err)

			sender := new(mocksender.MockSender)

			if !tt.disableAggregator {
				aggregator.InitAggregatorWithFlushInterval(nil, nil, "", 1*time.Hour)
			}

			mocksender.SetSender(sender, check.ID())

			session.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&tt.sysObjectIDPacket, tt.sysObjectIDError)
			session.On("Get", []string{"1.3.6.1.2.1.1.3.0", "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", "1.3.6.1.4.1.3375.2.1.1.2.1.44.999", "1.2.3.4.5", "1.3.6.1.2.1.1.5.0"}).Return(&tt.valuesPacket, tt.valuesError)

			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			sender.On("Commit").Return()

			err = check.Run()
			assert.EqualError(t, err, tt.expectedErr)

			snmpTags := []string{"snmp_device:1.2.3.4"}

			sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 0.0, "", snmpTags)
			sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpTags)
			sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpTags)

			sender.AssertServiceCheck(t, "snmp.can_check", metrics.ServiceCheckCritical, "", snmpTags, tt.expectedErr)
		})
	}
}

func TestCheck_Run_sessionCloseError(t *testing.T) {
	setConfdPathAndCleanProfiles()

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "debug")

	session := createMockSession()
	session.closeErr = fmt.Errorf("close error")
	check := Check{session: session}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
metrics:
- symbol:
    OID: 1.2.3
    name: myMetric
`)

	err = check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator

	mocksender.SetSender(sender, check.ID())

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{},
	}
	session.On("Get", []string{"1.2.3", "1.3.6.1.2.1.1.3.0"}).Return(&packet, nil)
	sender.SetupAcceptAll()

	err = check.Run()
	assert.Nil(t, err)

	w.Flush()
	logs := b.String()

	snmpTags := []string{"snmp_device:1.2.3.4"}
	sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 0.0, "", snmpTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpTags)
	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpTags)

	sender.AssertServiceCheck(t, "snmp.can_check", metrics.ServiceCheckOK, "", snmpTags, "")

	assert.Equal(t, strings.Count(logs, "failed to close session"), 1, logs)
}
