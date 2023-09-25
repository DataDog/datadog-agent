// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package snmp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/atomic"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/devicecheck"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/discovery"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

func demuxOpts() aggregator.AgentDemultiplexerOptions {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = 1 * time.Hour
	opts.DontStartForwarders = true
	return opts
}

type deps struct {
	fx.In
	Log       log.Component
	Forwarder defaultforwarder.Component
}

func createDeps(t *testing.T) deps {
	return fxutil.Test[deps](t, defaultforwarder.MockModule, config.MockModule, log.MockModule)
}

func Test_Run_simpleCase(t *testing.T) {
	deps := createDeps(t)
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}
	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
collect_topology: false
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

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()

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

	bulkBatch1Packet1 := gosnmp.SnmpPacket{
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
				Name:  "1.3.6.1.2.1.2.2.1.6.1",
				Type:  gosnmp.OctetString,
				Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o1},
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.7.1",
				Type:  gosnmp.Integer,
				Value: 1,
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
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.2",
				Type:  gosnmp.OctetString,
				Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o2},
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.7.2",
				Type:  gosnmp.Integer,
				Value: 3,
			},
		},
	}
	bulkBatch1Packet2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			// no more oids for batch 1
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
		},
	}

	bulkBatch2Packet1 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
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
				Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.1",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.1",
				Type:  gosnmp.IPAddress,
				Value: "255.255.255.0",
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
				Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.2",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.2",
				Type:  gosnmp.IPAddress,
				Value: "255.255.255.0",
			},
		},
	}

	bulkBatch2Packe2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			// no more matching oids for batch 2
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
		},
	}

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", mock.Anything).Return(&packet, nil)
	sess.On("Get", mock.Anything).Return(&packet, nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.14", "1.3.6.1.2.1.2.2.1.2", "1.3.6.1.2.1.2.2.1.20", "1.3.6.1.2.1.2.2.1.6", "1.3.6.1.2.1.2.2.1.7"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkBatch1Packet1, nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.14.2", "1.3.6.1.2.1.2.2.1.2.2", "1.3.6.1.2.1.2.2.1.20.2", "1.3.6.1.2.1.2.2.1.6.2", "1.3.6.1.2.1.2.2.1.7.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkBatch1Packet2, nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.8", "1.3.6.1.2.1.31.1.1.1.1", "1.3.6.1.2.1.31.1.1.1.18", "1.3.6.1.2.1.4.20.1.2", "1.3.6.1.2.1.4.20.1.3"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkBatch2Packet1, nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.8.2", "1.3.6.1.2.1.31.1.1.1.1.2", "1.3.6.1.2.1.31.1.1.1.18.2", "1.3.6.1.2.1.4.20.1.2.10.0.0.2", "1.3.6.1.2.1.4.20.1.3.10.0.0.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkBatch2Packe2, nil)

	err = chk.Run()
	assert.Nil(t, err)

	snmpTags := []string{"snmp_device:1.2.3.4"}
	snmpGlobalTags := append(common.CopyStrings(snmpTags), "snmp_host:foo_sys_name")
	snmpGlobalTagsWithLoader := append(common.CopyStrings(snmpGlobalTags), "loader:core")
	telemetryTags := append(common.CopyStrings(snmpGlobalTagsWithLoader), "agent_version:"+version.AgentVersion)
	row1Tags := append(common.CopyStrings(snmpGlobalTags), "if_index:1", "if_desc:desc1")
	row2Tags := append(common.CopyStrings(snmpGlobalTags), "if_index:2", "if_desc:desc2")
	scalarTags := append(common.CopyStrings(snmpGlobalTags), "symboltag1:1", "symboltag2:2")

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", snmpGlobalTags)
	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpGlobalTags)
	sender.AssertMetric(t, "Gauge", "snmp.ifNumber", float64(30), "", scalarTags)
	sender.AssertMetric(t, "Gauge", "snmp.aMetricWithExtractValue", float64(22), "", snmpGlobalTags)
	sender.AssertMetric(t, "Gauge", "snmp.ifInErrors", float64(141), "", row1Tags)
	sender.AssertMetric(t, "Gauge", "snmp.ifInErrors", float64(142), "", row2Tags)
	sender.AssertMetric(t, "Gauge", "snmp.ifOutErrors", float64(201), "", row1Tags)
	sender.AssertMetric(t, "Gauge", "snmp.ifOutErrors", float64(202), "", row2Tags)

	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", telemetryTags)
	sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 7, "", telemetryTags)

	chk.Cancel()
}

func Test_Run_customIfSpeed(t *testing.T) {
	deps := createDeps(t)
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}

	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
collect_device_metadata: false
interface_configs:
  - match_field: "name"
    match_value: "if10"
    in_speed: 50000000
    out_speed: 40000000
metrics:
- table:
    OID: 1.3.6.1.2.1.2.2
    name: ifTable
  metric_type: monotonic_count_and_rate
  symbols:
  - OID: 1.3.6.1.2.1.31.1.1.1.6
    name: ifHCInOctets
  - OID: 1.3.6.1.2.1.31.1.1.1.10
    name: ifHCOutOctets
  metric_tags:
  - column:
      OID: 1.3.6.1.2.1.31.1.1.1.1
      name: ifName
    tag: interface
- table:
    OID: 1.3.6.1.2.1.31.1.1
    name: ifXTable
  symbols:
  - OID: 1.3.6.1.2.1.31.1.1.1.15
    name: ifHighSpeed
  metric_tags:
  - column:
      OID: 1.3.6.1.2.1.31.1.1.1.1
      name: ifName
    tag: interface
`)

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.SetupAcceptAll()

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
		},
	}

	bulkBatch1Packet1 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
				Type:  gosnmp.OctetString,
				Value: []byte("if10"),
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.10.1",
				Type:  gosnmp.Integer,
				Value: 1_000_000,
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.15.1",
				Type:  gosnmp.Gauge32,
				Value: uint(100),
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.6.1",
				Type:  gosnmp.Integer,
				Value: 2_000_000,
			},
		},
	}
	bulkBatch1Packet2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name: "999",
				Type: gosnmp.NoSuchObject,
			},
		},
	}

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", mock.Anything).Return(&packet, nil)
	sess.On("Get", mock.Anything).Return(&packet, nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.31.1.1.1.1", "1.3.6.1.2.1.31.1.1.1.10", "1.3.6.1.2.1.31.1.1.1.15", "1.3.6.1.2.1.31.1.1.1.6"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkBatch1Packet1, nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.31.1.1.1.1.1", "1.3.6.1.2.1.31.1.1.1.10.1", "1.3.6.1.2.1.31.1.1.1.15.1", "1.3.6.1.2.1.31.1.1.1.6.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkBatch1Packet2, nil)

	err = chk.Run()
	assert.Nil(t, err)

	tags := []string{"snmp_device:1.2.3.4", "interface:if10"}
	sender.AssertMetric(t, "Gauge", "snmp.ifHighSpeed", float64(100), "", tags)
	sender.AssertMetric(t, "Gauge", "snmp.ifInSpeed", float64(50_000_000), "", tags)
	sender.AssertMetric(t, "Gauge", "snmp.ifOutSpeed", float64(40_000_000), "", tags)

	sender.AssertMetric(t, "MonotonicCount", "snmp.ifHCInOctets", float64(2_000_000), "", tags)
	sender.AssertMetric(t, "Rate", "snmp.ifHCInOctets.rate", float64(2_000_000), "", tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifHCOutOctets", float64(1_000_000), "", tags)
	sender.AssertMetric(t, "Rate", "snmp.ifHCOutOctets.rate", float64(1_000_000), "", tags)

	// ((2000000 * 8) / (50 * 1000000)) * 100 = 32
	//   ^ifHCInOctets   ^custom in_speed
	sender.AssertMetric(t, "Rate", "snmp.ifBandwidthInUsage.rate", float64(32), "", tags)
	// ((1000000 * 8) / (40 * 1000000)) * 100 = 20
	//   ^ifHCOutOctets   ^custom out_speed
	sender.AssertMetric(t, "Rate", "snmp.ifBandwidthOutUsage.rate", float64(20), "", tags)

	chk.Cancel()
}

func TestSupportedMetricTypes(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}
	// language=yaml
	rawInstanceConfig := []byte(`
collect_device_metadata: false
ip_address: 1.2.3.4
community_string: public
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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
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

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", mock.Anything).Return(&packet, nil)

	err = chk.Run()
	assert.Nil(t, err)

	tags := []string{"snmp_device:1.2.3.4"}
	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", tags)
	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", tags)
	sender.AssertMetric(t, "Gauge", "snmp.SomeGaugeMetric", float64(30), "", tags)
	sender.AssertMetric(t, "Rate", "snmp.SomeCounter32Metric", float64(40), "", tags)
	sender.AssertMetric(t, "Rate", "snmp.SomeCounter64Metric", float64(50), "", tags)
}

func TestProfile(t *testing.T) {
	timeNow = common.MockTimeNow

	deps := createDeps(t)
	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

	checkconfig.SetConfdPathAndCleanProfiles()

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
profile: f5-big-ip
collect_device_metadata: true
oid_batch_size: 10
collect_topology: false
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

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, rawInitConfig, "test")
	assert.NoError(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.2.3.4.5",
				Type:  gosnmp.Null,
				Value: nil,
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
			{
				Name: "1.3.6.1.4.1.3375.2.1.1.2.1.44.999",
				Type: gosnmp.Null,
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.3.3.3.0",
				Type:  gosnmp.OctetString,
				Value: []byte("a-serial-num"),
			},
		},
	}
	packetRetry := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.2.3.4.5.0",
				Type:  gosnmp.Null,
				Value: nil,
			},
			{
				Name: "1.3.6.1.4.1.3375.2.1.1.2.1.44.999.0",
				Type: gosnmp.Null,
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
				Name:  "1.3.6.1.2.1.2.2.1.6.1",
				Type:  gosnmp.OctetString,
				Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o1},
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
				Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.1",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.1",
				Type:  gosnmp.IPAddress,
				Value: "255.255.255.0",
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
				Name:  "1.3.6.1.2.1.2.2.1.6.2",
				Type:  gosnmp.OctetString,
				Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o2},
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
				Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.2",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.2",
				Type:  gosnmp.IPAddress,
				Value: "255.255.255.0",
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

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", []string{
		"1.2.3.4.5",
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.1.5.0",
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.999",
		"1.3.6.1.4.1.3375.2.1.3.3.3.0",
	}).Return(&packet, nil)
	sess.On("Get", []string{
		"1.2.3.4.5.0",
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.999.0",
	}).Return(&packetRetry, nil)
	sess.On("GetBulk", []string{
		"1.3.6.1.2.1.2.2.1.13",
		"1.3.6.1.2.1.2.2.1.14",
		"1.3.6.1.2.1.2.2.1.6",
		"1.3.6.1.2.1.2.2.1.7",
		"1.3.6.1.2.1.2.2.1.8",
		"1.3.6.1.2.1.31.1.1.1.1",
		"1.3.6.1.2.1.31.1.1.1.18",
		"1.3.6.1.2.1.4.20.1.2",
		"1.3.6.1.2.1.4.20.1.3",
	}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket, nil)

	err = chk.Run()
	assert.Nil(t, err)

	snmpTags := []string{
		"device_namespace:default",
		"snmp_device:1.2.3.4",
		"snmp_profile:f5-big-ip",
		"device_vendor:f5",
		"snmp_host:foo_sys_name",
		"static_tag:from_profile_root",
		"static_tag:from_base_profile",
	}
	row1Tags := append(common.CopyStrings(snmpTags), "interface:nameRow1", "interface_alias:descRow1", "mac_address:00:00:00:00:00:01", "table_static_tag:val")
	row2Tags := append(common.CopyStrings(snmpTags), "interface:nameRow2", "interface_alias:descRow2", "mac_address:00:00:00:00:00:02", "table_static_tag:val")

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", snmpTags)
	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpTags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(70.5), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(71), "", row2Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(131), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(132), "", row2Tags)
	sender.AssertMetric(t, "Gauge", "snmp.sysStatMemoryTotal", float64(60), "", snmpTags)

	// language=json
	event := []byte(fmt.Sprintf(`
{
  "subnet": "127.0.0.0/30",
  "namespace":"default",
  "devices": [
    {
      "id": "default:1.2.3.4",
      "id_tags": [
        "device_namespace:default",
        "snmp_device:1.2.3.4"
      ],
      "tags": [
        "agent_version:%s",
        "autodiscovery_subnet:127.0.0.0/30",
        "device_namespace:default",
        "device_vendor:f5",
        "mytag:val1",
        "prefix:f",
        "snmp_device:1.2.3.4",
        "snmp_host:foo_sys_name",
        "snmp_profile:f5-big-ip",
        "some_tag:some_tag_value",
        "static_tag:from_base_profile",
        "static_tag:from_profile_root",
        "suffix:oo_sys_name"
      ],
      "ip_address": "1.2.3.4",
      "status": 1,
      "name": "foo_sys_name",
      "description": "my_desc",
      "sys_object_id": "1.2.3.4",
      "profile": "f5-big-ip",
      "vendor": "f5",
      "subnet": "127.0.0.0/30",
      "serial_number": "a-serial-num"
    }
  ],
  "interfaces": [
    {
      "device_id": "default:1.2.3.4",
      "id_tags": ["custom-tag:nameRow1","interface:nameRow1"],
      "index": 1,
      "name": "nameRow1",
      "alias": "descRow1",
      "description": "Row1",
      "mac_address": "00:00:00:00:00:01",
      "admin_status": 1,
      "oper_status": 1
    },
    {
      "device_id": "default:1.2.3.4",
	  "id_tags": ["custom-tag:nameRow2","interface:nameRow2"],
      "index": 2,
      "name": "nameRow2",
      "alias": "descRow2",
      "description": "Row2",
      "mac_address": "00:00:00:00:00:02",
      "admin_status": 1,
      "oper_status": 1
    }
  ],
  "ip_addresses": [
    {
      "interface_id": "default:1.2.3.4:1",
      "ip_address": "10.0.0.1",
      "prefixlen": 24
    },
    {
      "interface_id": "default:1.2.3.4:1",
      "ip_address": "10.0.0.2",
      "prefixlen": 24
    }
  ],
  "diagnoses": [
    {
      "resource_type": "device",
      "resource_id": "default:1.2.3.4",
      "diagnoses": null
    }
  ],
  "collect_timestamp":946684800
}
`, version.AgentVersion))
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")

	sender.AssertServiceCheck(t, "snmp.can_check", servicecheck.ServiceCheckOK, "", snmpTags, "")
}

func TestServiceCheckFailures(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	sess.ConnectErr = fmt.Errorf("can't connect")
	chk := Check{sessionFactory: sessionFactory}

	// language=yaml
	rawInstanceConfig := []byte(`
collect_device_metadata: false
ip_address: 1.2.3.4
community_string: public
`)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	err = chk.Run()
	assert.Error(t, err, "snmp connection error: can't connect")

	snmpTags := []string{"snmp_device:1.2.3.4"}

	sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 0.0, "", snmpTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpTags)
	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpTags)
	sender.AssertServiceCheck(t, "snmp.can_check", servicecheck.ServiceCheckCritical, "", snmpTags, "snmp connection error: can't connect")
}

func TestCheckID(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	check1 := snmpFactory()
	check2 := snmpFactory()
	check3 := snmpFactory()
	checkSubnet := snmpFactory()
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
	// language=yaml
	rawInstanceConfig3 := []byte(`
ip_address: 3.3.3.3
community_string: abc
namespace: ns3
`)
	// language=yaml
	rawInstanceConfigSubnet := []byte(`
network_address: 10.10.10.0/24
community_string: abc
namespace: nsSubnet
`)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := check1.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig1, []byte(``), "test")
	assert.Nil(t, err)
	err = check2.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig2, []byte(``), "test")
	assert.Nil(t, err)
	err = check3.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig3, []byte(``), "test")
	assert.Nil(t, err)
	err = checkSubnet.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfigSubnet, []byte(``), "test")
	assert.Nil(t, err)

	assert.Equal(t, checkid.ID("snmp:default:1.1.1.1:9d3f14dbaceba72d"), check1.ID())
	assert.Equal(t, checkid.ID("snmp:default:2.2.2.2:9c51b342e7a4fdd5"), check2.ID())
	assert.Equal(t, checkid.ID("snmp:ns3:3.3.3.3:7e1c698677986eca"), check3.ID())
	assert.Equal(t, checkid.ID("snmp:nsSubnet:10.10.10.0/24:ae80a9e88fe6643e"), checkSubnet.ID())
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
				Value: 1234,
			},
		},
	}

	sysObjectIDPacketInvalidOidMock := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.6.3.15.1.1.1.0", // usmStatsUnsupportedSecLevels
				Type:  gosnmp.Counter32,
				Value: 123,
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

	valuesPacketUptime := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
		},
	}

	tests := []struct {
		name                     string
		sessionConnError         error
		sysObjectIDPacket        gosnmp.SnmpPacket
		sysObjectIDError         error
		reachableGetNextError    error
		reachableValuesPacket    gosnmp.SnmpPacket
		valuesPacket             gosnmp.SnmpPacket
		valuesError              error
		expectedErr              string
		expectedSubmittedMetrics float64
	}{
		{
			name:             "connection error",
			sessionConnError: fmt.Errorf("can't connect"),
			expectedErr:      "snmp connection error: can't connect",
		},
		{
			name:                     "failed to fetch sysobjectid",
			sysObjectIDError:         fmt.Errorf("no sysobjectid"),
			valuesPacket:             valuesPacketUptime,
			reachableValuesPacket:    gosnmplib.MockValidReachableGetNextPacket,
			expectedErr:              "failed to autodetect profile: failed to fetch sysobjectid: cannot get sysobjectid: no sysobjectid",
			expectedSubmittedMetrics: 1.0,
		},
		{
			name:                  "unexpected values count",
			reachableValuesPacket: gosnmplib.MockValidReachableGetNextPacket,
			expectedErr:           "failed to autodetect profile: failed to fetch sysobjectid: expected 1 value, but got 0: variables=[]",
		},
		{
			name:                  "failed to fetch sysobjectid with invalid value",
			reachableValuesPacket: gosnmplib.MockValidReachableGetNextPacket,
			sysObjectIDPacket:     sysObjectIDPacketInvalidValueMock,
			expectedErr:           "failed to autodetect profile: failed to fetch sysobjectid: error getting value from pdu: oid 1.3.6.1.2.1.1.2.0: ObjectIdentifier should be string type but got type `float64` and value `1`",
		},
		{
			name:                  "failed to fetch sysobjectid with conversion error",
			reachableValuesPacket: gosnmplib.MockValidReachableGetNextPacket,
			sysObjectIDPacket:     sysObjectIDPacketInvalidConversionMock,
			expectedErr:           "failed to autodetect profile: failed to fetch sysobjectid: error getting value from pdu: oid 1.3.6.1.2.1.1.2.0: ObjectIdentifier should be string type but got type `int` and value `1234`",
		},
		{
			name:                  "failed to fetch sysobjectid with error oid",
			reachableValuesPacket: gosnmplib.MockValidReachableGetNextPacket,
			sysObjectIDPacket:     sysObjectIDPacketInvalidOidMock,
			expectedErr:           "failed to autodetect profile: failed to fetch sysobjectid: expect `1.3.6.1.2.1.1.2.0` OID but got `1.3.6.1.6.3.15.1.1.1.0` OID with value `{counter 123}`",
		},
		{
			name:                  "failed to get profile sys object id",
			reachableValuesPacket: gosnmplib.MockValidReachableGetNextPacket,
			sysObjectIDPacket:     sysObjectIDPacketInvalidSysObjectIDMock,
			expectedErr:           "failed to autodetect profile: failed to get profile sys object id for `1.999999`: failed to get most specific profile for sysObjectID `1.999999`, for matched oids []: cannot get most specific oid from empty list of oids",
		},
		{
			name:                  "failed to fetch values",
			reachableValuesPacket: gosnmplib.MockValidReachableGetNextPacket,
			sysObjectIDPacket:     sysObjectIDPacketOkMock,
			valuesPacket:          valuesPacketErrMock,
			valuesError:           fmt.Errorf("no value"),
			expectedErr:           "failed to fetch values: failed to fetch scalar oids with batching: failed to fetch scalar oids: fetch scalar: error getting oids `[1.2.3.4.5 1.3.6.1.2.1.1.3.0 1.3.6.1.2.1.1.5.0 1.3.6.1.4.1.3375.2.1.1.2.1.44.0 1.3.6.1.4.1.3375.2.1.1.2.1.44.999]`: no value",
		},
		{
			name:                  "failed to fetch sysobjectid and failed to fetch values",
			reachableValuesPacket: gosnmplib.MockValidReachableGetNextPacket,
			sysObjectIDError:      fmt.Errorf("no sysobjectid"),
			valuesPacket:          valuesPacketErrMock,
			valuesError:           fmt.Errorf("no value"),
			expectedErr:           "failed to autodetect profile: failed to fetch sysobjectid: cannot get sysobjectid: no sysobjectid; failed to fetch values: failed to fetch scalar oids with batching: failed to fetch scalar oids: fetch scalar: error getting oids `[1.3.6.1.2.1.1.3.0]`: no value",
		},
		{
			name:                  "failed reachability check",
			sysObjectIDError:      fmt.Errorf("no sysobjectid"),
			reachableGetNextError: fmt.Errorf("no value for GextNext"),
			valuesPacket:          valuesPacketErrMock,
			valuesError:           fmt.Errorf("no value"),
			expectedErr:           "check device reachable: failed: no value for GextNext; failed to autodetect profile: failed to fetch sysobjectid: cannot get sysobjectid: no sysobjectid; failed to fetch values: failed to fetch scalar oids with batching: failed to fetch scalar oids: fetch scalar: error getting oids `[1.3.6.1.2.1.1.3.0]`: no value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkconfig.SetConfdPathAndCleanProfiles()
			sess := session.CreateMockSession()
			sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
				return sess, nil
			}
			sess.ConnectErr = tt.sessionConnError
			chk := Check{sessionFactory: sessionFactory}

			// language=yaml
			rawInstanceConfig := []byte(fmt.Sprintf(`
collect_device_metadata: false
ip_address: 1.2.3.4
community_string: public
namespace: '%s'
`, tt.name))
			deps := createDeps(t)
			senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

			err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
			assert.Nil(t, err)

			sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)

			sess.On("GetNext", []string{"1.0"}).Return(&tt.reachableValuesPacket, tt.reachableGetNextError)
			sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&tt.sysObjectIDPacket, tt.sysObjectIDError)
			sess.On("Get", []string{"1.2.3.4.5", "1.3.6.1.2.1.1.3.0", "1.3.6.1.2.1.1.5.0", "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", "1.3.6.1.4.1.3375.2.1.1.2.1.44.999"}).Return(&tt.valuesPacket, tt.valuesError)
			sess.On("Get", []string{"1.3.6.1.2.1.1.3.0"}).Return(&tt.valuesPacket, tt.valuesError)

			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			sender.On("Commit").Return()

			err = chk.Run()
			assert.EqualError(t, err, tt.expectedErr)

			snmpTags := []string{"snmp_device:1.2.3.4"}

			sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", tt.expectedSubmittedMetrics, "", snmpTags)
			sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpTags)
			sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpTags)

			sender.AssertServiceCheck(t, "snmp.can_check", servicecheck.ServiceCheckCritical, "", snmpTags, tt.expectedErr)
		})
		break
	}
}

func TestCheck_Run_sessionCloseError(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	sess.CloseErr = fmt.Errorf("close error")
	chk := Check{sessionFactory: sessionFactory}

	// language=yaml
	rawInstanceConfig := []byte(`
collect_device_metadata: false
ip_address: 1.2.3.4
community_string: public
metrics:
- symbol:
    OID: 1.2.3
    name: myMetric
`)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{},
	}
	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", []string{"1.2.3", "1.3.6.1.2.1.1.3.0"}).Return(&packet, nil)
	sender.SetupAcceptAll()

	err = chk.Run()
	assert.Nil(t, err)

	snmpTags := []string{"snmp_device:1.2.3.4"}
	sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 0.0, "", snmpTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpTags)
	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpTags)

	sender.AssertServiceCheck(t, "snmp.can_check", servicecheck.ServiceCheckOK, "", snmpTags, "")
}

func TestReportDeviceMetadataEvenOnProfileError(t *testing.T) {
	timeNow = common.MockTimeNow

	deps := createDeps(t)
	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")
	checkconfig.SetConfdPathAndCleanProfiles()

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
collect_device_metadata: true
oid_batch_size: 10
collect_topology: false
tags:
  - "mytag:val1"
  - "autodiscovery_subnet:127.0.0.0/30"
`)
	// language=yaml
	rawInitConfig := []byte(``)

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
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
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
		},
	}

	bulkPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.2.2.1.2.1",
				Type:  gosnmp.OctetString,
				Value: []byte("ifDescRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.1",
				Type:  gosnmp.OctetString,
				Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o1},
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
				Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.1",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.1",
				Type:  gosnmp.IPAddress,
				Value: "255.255.255.0",
			},

			{
				Name:  "1.3.6.1.2.1.2.2.1.2.2",
				Type:  gosnmp.OctetString,
				Value: []byte("ifDescRow2"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.2",
				Type:  gosnmp.OctetString,
				Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o2},
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
				Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.2",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.2",
				Type:  gosnmp.IPAddress,
				Value: "255.255.255.0",
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
	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	var sysObjectIDPacket *gosnmp.SnmpPacket
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(sysObjectIDPacket, fmt.Errorf("no value"))

	sess.On("Get", []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.1.5.0",
	}).Return(&packet, nil)
	sess.On("GetBulk", []string{
		//"1.3.6.1.2.1.2.2.1.13",
		//"1.3.6.1.2.1.2.2.1.14",
		"1.3.6.1.2.1.2.2.1.2",
		"1.3.6.1.2.1.2.2.1.6",
		"1.3.6.1.2.1.2.2.1.7",
		"1.3.6.1.2.1.2.2.1.8",
		"1.3.6.1.2.1.31.1.1.1.1",
		"1.3.6.1.2.1.31.1.1.1.18",
		"1.3.6.1.2.1.4.20.1.2",
		"1.3.6.1.2.1.4.20.1.3",
	}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket, nil)

	err = chk.Run()
	assert.EqualError(t, err, "failed to autodetect profile: failed to fetch sysobjectid: cannot get sysobjectid: no value")

	snmpTags := []string{"device_namespace:default", "snmp_device:1.2.3.4"}

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", snmpTags)
	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpTags)

	// language=json
	event := []byte(fmt.Sprintf(`
{
  "subnet": "127.0.0.0/30",
  "namespace":"default",
  "devices": [
    {
      "id": "default:1.2.3.4",
      "id_tags": [
        "device_namespace:default",
        "snmp_device:1.2.3.4"
      ],
      "tags": [
        "agent_version:%s",
        "autodiscovery_subnet:127.0.0.0/30",
        "device_namespace:default",
        "mytag:val1",
        "snmp_device:1.2.3.4"
      ],
      "ip_address": "1.2.3.4",
      "status": 1,
      "name": "foo_sys_name",
      "description": "my_desc",
      "sys_object_id": "1.2.3.4",
      "subnet": "127.0.0.0/30"
    }
  ],
  "interfaces": [
    {
      "device_id": "default:1.2.3.4",
      "id_tags": ["interface:nameRow1"],
      "index": 1,
      "name": "nameRow1",
      "alias": "descRow1",
      "description": "ifDescRow1",
      "mac_address": "00:00:00:00:00:01",
      "admin_status": 1,
      "oper_status": 1
    },
    {
      "device_id": "default:1.2.3.4",
      "id_tags": ["interface:nameRow2"],
      "index": 2,
      "name": "nameRow2",
      "alias": "descRow2",
      "description": "ifDescRow2",
      "mac_address": "00:00:00:00:00:02",
      "admin_status": 1,
      "oper_status": 1
    }
  ],
  "ip_addresses": [
    {
      "interface_id": "default:1.2.3.4:1",
      "ip_address": "10.0.0.1",
      "prefixlen": 24
    },
    {
      "interface_id": "default:1.2.3.4:1",
      "ip_address": "10.0.0.2",
      "prefixlen": 24
    }
  ],
  "diagnoses": [
    {
      "resource_type": "device",
      "resource_id": "default:1.2.3.4",
      "diagnoses": [
        {
          "severity": "error",
          "message": "Agent failed to detect a profile for this network device.",
          "code": "SNMP_FAILED_TO_DETECT_PROFILE"
        }
      ]
    }
  ],
  "collect_timestamp":946684800
}
`, version.AgentVersion))
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")

	sender.AssertServiceCheck(t, "snmp.can_check", servicecheck.ServiceCheckCritical, "", snmpTags, "failed to autodetect profile: failed to fetch sysobjectid: cannot get sysobjectid: no value")
}

func TestReportDeviceMetadataWithFetchError(t *testing.T) {
	timeNow = common.MockTimeNow
	deps := createDeps(t)
	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

	checkconfig.SetConfdPathAndCleanProfiles()

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.5
community_string: public
collect_device_metadata: true
tags:
  - "mytag:val1"
  - "autodiscovery_subnet:127.0.0.0/30"
`)
	// language=yaml
	rawInitConfig := []byte(``)

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, rawInitConfig, "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	var nilPacket *gosnmp.SnmpPacket
	sess.On("GetNext", []string{"1.0"}).Return(nilPacket, fmt.Errorf("no value for GetNext"))
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(nilPacket, fmt.Errorf("no value"))

	sess.On("Get", []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.1.5.0",
	}).Return(nilPacket, fmt.Errorf("device failure"))

	expectedErrMsg := "check device reachable: failed: no value for GetNext; failed to autodetect profile: failed to fetch sysobjectid: cannot get sysobjectid: no value; failed to fetch values: failed to fetch scalar oids with batching: failed to fetch scalar oids: fetch scalar: error getting oids `[1.3.6.1.2.1.1.1.0 1.3.6.1.2.1.1.2.0 1.3.6.1.2.1.1.3.0 1.3.6.1.2.1.1.5.0]`: device failure"

	err = chk.Run()
	assert.EqualError(t, err, expectedErrMsg)

	snmpTags := []string{"device_namespace:default", "snmp_device:1.2.3.5"}

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", snmpTags)

	// language=json
	event := []byte(fmt.Sprintf(`
{
  "subnet": "127.0.0.0/30",
  "namespace":"default",
  "devices": [
    {
      "id": "default:1.2.3.5",
      "id_tags": [
        "device_namespace:default",
        "snmp_device:1.2.3.5"
      ],
      "tags": [
        "agent_version:%s",
        "autodiscovery_subnet:127.0.0.0/30",
        "device_namespace:default",
        "mytag:val1",
        "snmp_device:1.2.3.5"
      ],
      "ip_address": "1.2.3.5",
      "status": 2,
      "subnet": "127.0.0.0/30"
    }
  ],
  "diagnoses": [
	{
	  "resource_type": "device",
	  "resource_id": "default:1.2.3.5",
	  "diagnoses": [
		{
		  "severity": "error",
		  "message": "Agent failed to poll this network device. Check the authentication method and ensure the agent can ping it.",
		  "code": "SNMP_FAILED_TO_POLL_DEVICE"
		},
		{
		  "severity": "error",
		  "message": "Agent failed to detect a profile for this network device.",
		  "code": "SNMP_FAILED_TO_DETECT_PROFILE"
		}
	  ]
	}
  ],
  "collect_timestamp":946684800
}
`, version.AgentVersion))
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")

	sender.AssertServiceCheck(t, "snmp.can_check", servicecheck.ServiceCheckCritical, "", snmpTags, expectedErrMsg)
}

func TestDiscovery(t *testing.T) {
	deps := createDeps(t)
	timeNow = common.MockTimeNow
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}
	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

	// language=yaml
	rawInstanceConfig := []byte(`
network_address: 10.10.0.0/30
community_string: public
collect_device_metadata: true
oid_batch_size: 10
collect_topology: false
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: ifNumber
  metric_tags:
  - symboltag1:1
  - symboltag2:2
metric_tags:
  - OID: 1.3.6.1.2.1.1.5.0
    symbol: sysName
    tag: snmp_host
`)

	discoveryPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.2.3",
			},
		},
	}

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&discoveryPacket, nil)

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	_, err = waitForDiscoveredDevices(chk.discovery, 4, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
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
		},
	}

	sess.On("Get", mock.Anything).Return(&packet, nil)

	bulkPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.2.2.1.2.1",
				Type:  gosnmp.OctetString,
				Value: []byte("ifDescRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.1",
				Type:  gosnmp.OctetString,
				Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o1},
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
				Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.1",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.1",
				Type:  gosnmp.IPAddress,
				Value: "255.255.255.0",
			},

			{
				Name:  "1.3.6.1.2.1.2.2.1.2.2",
				Type:  gosnmp.OctetString,
				Value: []byte("ifDescRow2"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.2",
				Type:  gosnmp.OctetString,
				Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o2},
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
				Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.2",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.2",
				Type:  gosnmp.IPAddress,
				Value: "255.255.255.0",
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

	sess.On("GetBulk", []string{
		//"1.3.6.1.2.1.2.2.1.2", "1.3.6.1.2.1.2.2.1.6", "1.3.6.1.2.1.2.2.1.7", "1.3.6.1.2.1.2.2.1.8", "1.3.6.1.2.1.31.1.1.1.1"
		"1.3.6.1.2.1.2.2.1.2",
		"1.3.6.1.2.1.2.2.1.6",
		"1.3.6.1.2.1.2.2.1.7",
		"1.3.6.1.2.1.2.2.1.8",
		"1.3.6.1.2.1.31.1.1.1.1",
		"1.3.6.1.2.1.31.1.1.1.18",
		"1.3.6.1.2.1.4.20.1.2",
		"1.3.6.1.2.1.4.20.1.3",
	}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket, nil)

	deviceMap := []struct {
		ipAddress string
		deviceID  string
	}{
		{ipAddress: "10.10.0.0", deviceID: "default:10.10.0.0"},
		{ipAddress: "10.10.0.1", deviceID: "default:10.10.0.1"},
		{ipAddress: "10.10.0.2", deviceID: "default:10.10.0.2"},
		{ipAddress: "10.10.0.3", deviceID: "default:10.10.0.3"},
	}

	err = chk.Run()
	assert.Nil(t, err)

	for _, deviceData := range deviceMap {
		snmpTags := []string{"device_namespace:default", "snmp_device:" + deviceData.ipAddress, "autodiscovery_subnet:10.10.0.0/30"}
		snmpGlobalTags := append(common.CopyStrings(snmpTags), "snmp_host:foo_sys_name")
		snmpGlobalTagsWithLoader := append(common.CopyStrings(snmpGlobalTags), "loader:core")
		scalarTags := append(common.CopyStrings(snmpGlobalTags), "symboltag1:1", "symboltag2:2")

		sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", snmpGlobalTags)
		sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpGlobalTags)
		sender.AssertMetric(t, "Gauge", "snmp.ifNumber", float64(30), "", scalarTags)

		sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpGlobalTagsWithLoader)
		sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpGlobalTagsWithLoader)
		sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 2, "", snmpGlobalTagsWithLoader)

		// language=json
		event := []byte(fmt.Sprintf(`
{
  "subnet": "10.10.0.0/30",
  "namespace":"default",
  "devices": [
    {
      "id": "%s",
      "id_tags": [
        "device_namespace:default",
        "snmp_device:%s"
      ],
      "tags": [
        "agent_version:%s",
        "autodiscovery_subnet:10.10.0.0/30",
        "device_namespace:default",
        "snmp_device:%s",
        "snmp_host:foo_sys_name"
      ],
      "ip_address": "%s",
      "status": 1,
      "name": "foo_sys_name",
      "subnet": "10.10.0.0/30"
    }
  ],
  "interfaces": [
    {
      "device_id": "%s",
      "id_tags": ["interface:nameRow1"],
      "index": 1,
      "name": "nameRow1",
      "alias": "descRow1",
      "description": "ifDescRow1",
      "mac_address": "00:00:00:00:00:01",
      "admin_status": 1,
      "oper_status": 1
    },
    {
      "device_id": "%s",
	  "id_tags": ["interface:nameRow2"],
      "index": 2,
      "name": "nameRow2",
      "alias": "descRow2",
      "description": "ifDescRow2",
      "mac_address": "00:00:00:00:00:02",
      "admin_status": 1,
      "oper_status": 1
    }
  ],
  "ip_addresses": [
    {
      "interface_id": "%s:1",
      "ip_address": "10.0.0.1",
      "prefixlen": 24
    },
    {
      "interface_id": "%s:1",
      "ip_address": "10.0.0.2",
      "prefixlen": 24
    }
  ],
  "diagnoses": [
    {
      "resource_type": "device",
      "resource_id": "%s",
      "diagnoses": null
    }
  ],
  "collect_timestamp":946684800
}
`, deviceData.deviceID, deviceData.ipAddress, version.AgentVersion, deviceData.ipAddress, deviceData.ipAddress, deviceData.deviceID, deviceData.deviceID, deviceData.deviceID, deviceData.deviceID, deviceData.deviceID))
		compactEvent := new(bytes.Buffer)
		err = json.Compact(compactEvent, event)
		assert.NoError(t, err)

		sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-metadata")
	}
	networkTags := []string{"network:10.10.0.0/30", "autodiscovery_subnet:10.10.0.0/30"}
	sender.AssertMetric(t, "Gauge", "snmp.discovered_devices_count", 4, "", networkTags)

	chk.Cancel()
	assert.Nil(t, chk.discovery)
}

func TestDiscovery_CheckError(t *testing.T) {
	deps := createDeps(t)
	checkconfig.SetConfdPathAndCleanProfiles()

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory, workerRunDeviceCheckErrors: atomic.NewUint64(0)}
	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

	// language=yaml
	rawInstanceConfig := []byte(`
collect_device_metadata: false
network_address: 10.10.0.0/30
community_string: public
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: ifNumber
  metric_tags:
  - symboltag1:1
  - symboltag2:2
metric_tags:
  - OID: 1.3.6.1.2.1.1.5.0
    symbol: sysName
    tag: snmp_host
`)

	discoveryPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.2.3",
			},
		},
	}

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&discoveryPacket, nil)

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	_, err = waitForDiscoveredDevices(chk.discovery, 4, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	sess.On("Get", mock.Anything).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("get error"))

	err = chk.Run()
	assert.Nil(t, err)

	for i := 0; i < 4; i++ {
		snmpTags := []string{fmt.Sprintf("snmp_device:10.10.0.%d", i)}
		snmpGlobalTags := common.CopyStrings(snmpTags)
		snmpGlobalTagsWithLoader := append(common.CopyStrings(snmpGlobalTags), "loader:core")

		sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", snmpGlobalTags)
		sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpGlobalTagsWithLoader)
		sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpGlobalTagsWithLoader)
		sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 0, "", snmpGlobalTagsWithLoader)
	}

	assert.Equal(t, uint64(4), chk.workerRunDeviceCheckErrors.Load())
}

func TestDeviceIDAsHostname(t *testing.T) {
	deps := createDeps(t)
	cache.Cache.Delete(cache.BuildAgentKey("hostname")) // clean existing hostname cache

	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}
	coreconfig.Datadog.Set("hostname", "test-hostname")
	coreconfig.Datadog.Set("tags", []string{"agent_tag1:val1", "agent_tag2:val2"})
	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
collect_topology: false
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: ifNumber
  metric_tags:
  - symboltag1:1
  - symboltag2:2
use_device_id_as_hostname: true
`)

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
			{
				Name:  "1.3.6.1.2.1.2.1",
				Type:  gosnmp.Integer,
				Value: 30,
			},
		},
	}

	sess.On("Get", mock.Anything).Return(&packet, nil)

	bulkPackets := []gosnmp.SnmpPacket{
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.3.6.1.2.1.2.2.1.2.1",
					Type:  gosnmp.OctetString,
					Value: []byte("ifDescRow1"),
				},
				{
					Name:  "1.3.6.1.2.1.2.2.1.6.1",
					Type:  gosnmp.OctetString,
					Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o1},
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
		}, {
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.3.6.1.2.1.31.1.1.1.18.1",
					Type:  gosnmp.OctetString,
					Value: []byte("ifDescRow1"),
				},
				{
					Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.1",
					Type:  gosnmp.Integer,
					Value: 1,
				},
				{
					Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.1",
					Type:  gosnmp.IPAddress,
					Value: "255.255.255.0",
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
		},
	}
	sess.On("GetBulk", []string{
		"1.3.6.1.2.1.2.2.1.2", "1.3.6.1.2.1.2.2.1.6", "1.3.6.1.2.1.2.2.1.7", "1.3.6.1.2.1.2.2.1.8", "1.3.6.1.2.1.31.1.1.1.1",
	}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPackets[0], nil)
	sess.On("GetBulk", []string{
		"1.3.6.1.2.1.31.1.1.1.18",
		"1.3.6.1.2.1.4.20.1.2",
		"1.3.6.1.2.1.4.20.1.3",
	}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPackets[1], nil)

	err = chk.Run()
	assert.Nil(t, err)

	hostname := "device:default:1.2.3.4"
	snmpTags := []string{"snmp_device:1.2.3.4"}
	snmpGlobalTags := common.CopyStrings(snmpTags)
	snmpGlobalTagsWithLoader := append(common.CopyStrings(snmpGlobalTags), "loader:core")
	scalarTags := append(common.CopyStrings(snmpGlobalTags), "symboltag1:1", "symboltag2:2")

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), hostname, snmpGlobalTags)
	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), hostname, snmpGlobalTags)
	sender.AssertMetric(t, "Gauge", "snmp.ifNumber", float64(30), hostname, scalarTags)

	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpGlobalTagsWithLoader)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpGlobalTagsWithLoader)
	sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 2, hostname, snmpGlobalTagsWithLoader)

	// Test SetExternalTags
	host := "device:default:1.2.3.4"
	sourceType := "snmp"
	externalTags := []string{"agent_tag1:val1", "agent_tag2:val2"}
	eTags := externalhost.ExternalTags{sourceType: externalTags}

	p := *externalhost.GetPayload()
	var hostTags []interface{}
	for _, curHostTags := range p {
		if curHostTags[0].(string) == host {
			hostTags = curHostTags
		}
	}
	assert.Contains(t, hostTags, host)
	assert.Contains(t, hostTags, eTags)
}

func TestDiscoveryDeviceIDAsHostname(t *testing.T) {
	deps := createDeps(t)
	cache.Cache.Delete(cache.BuildAgentKey("hostname")) // clean existing hostname cache
	timeNow = common.MockTimeNow
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}

	coreconfig.Datadog.Set("hostname", "my-hostname")
	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

	// language=yaml
	rawInstanceConfig := []byte(`
network_address: 10.10.0.0/30
community_string: public
use_device_id_as_hostname: true
oid_batch_size: 10
collect_topology: false
metrics:
- symbol:
    OID: 1.3.6.1.2.1.2.1
    name: ifNumber
  metric_tags:
  - symboltag1:1
  - symboltag2:2
`)

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)

	discoveryPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.2.3",
			},
		},
	}
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&discoveryPacket, nil)

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	_, err = waitForDiscoveredDevices(chk.discovery, 4, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
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
		},
	}
	sess.On("Get", mock.Anything).Return(&packet, nil)

	bulkPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.2.2.1.2.1",
				Type:  gosnmp.OctetString,
				Value: []byte("ifDescRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.1",
				Type:  gosnmp.OctetString,
				Value: []byte{0o0, 0o0, 0o0, 0o0, 0o0, 0o1},
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
				Value: []byte("ifDescRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.2.10.0.0.1",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.4.20.1.3.10.0.0.1",
				Type:  gosnmp.IPAddress,
				Value: "255.255.255.0",
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
	sess.On("GetBulk", []string{
		"1.3.6.1.2.1.2.2.1.2",
		"1.3.6.1.2.1.2.2.1.6",
		"1.3.6.1.2.1.2.2.1.7",
		"1.3.6.1.2.1.2.2.1.8",
		"1.3.6.1.2.1.31.1.1.1.1",
		"1.3.6.1.2.1.31.1.1.1.18",
		"1.3.6.1.2.1.4.20.1.2",
		"1.3.6.1.2.1.4.20.1.3",
	}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket, nil)

	deviceMap := []struct {
		ipAddress string
		deviceID  string
	}{
		{ipAddress: "10.10.0.0", deviceID: "default:10.10.0.0"},
		{ipAddress: "10.10.0.1", deviceID: "default:10.10.0.1"},
		{ipAddress: "10.10.0.2", deviceID: "default:10.10.0.2"},
		{ipAddress: "10.10.0.3", deviceID: "default:10.10.0.3"},
	}

	err = chk.Run()
	assert.Nil(t, err)

	for _, deviceData := range deviceMap {
		hostname := "device:" + deviceData.deviceID
		snmpTags := []string{"snmp_device:" + deviceData.ipAddress, "autodiscovery_subnet:10.10.0.0/30", "agent_host:my-hostname"}
		snmpGlobalTags := common.CopyStrings(snmpTags)
		snmpGlobalTagsWithLoader := append(common.CopyStrings(snmpGlobalTags), "loader:core")
		scalarTags := append(common.CopyStrings(snmpGlobalTags), "symboltag1:1", "symboltag2:2")

		sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), hostname, snmpGlobalTags)
		sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), hostname, snmpGlobalTags)
		sender.AssertMetric(t, "Gauge", "snmp.ifNumber", float64(30), hostname, scalarTags)

		sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", snmpGlobalTagsWithLoader)
		sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", snmpGlobalTagsWithLoader)
		sender.AssertMetric(t, "Gauge", "datadog.snmp.submitted_metrics", 2, hostname, snmpGlobalTagsWithLoader)
	}
	networkTags := []string{"network:10.10.0.0/30", "autodiscovery_subnet:10.10.0.0/30"}
	sender.AssertMetric(t, "Gauge", "snmp.discovered_devices_count", 4, "", networkTags)
}

func TestCheckCancel(t *testing.T) {
	deps := createDeps(t)
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}

	senderManager := aggregator.InitAndStartAgentDemultiplexer(deps.Log, deps.Forwarder, demuxOpts(), "")

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
`)

	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	// check Cancel does not panic when called with single check
	// it shouldn't try to stop discovery
	chk.Cancel()
}

// Wait for discovery to be completed
func waitForDiscoveredDevices(discovery *discovery.Discovery, expectedDeviceCount int, timeout time.Duration) ([]*devicecheck.DeviceCheck, error) {
	timeoutTimer := time.After(timeout)
	tick := time.Tick(100 * time.Millisecond)

	for {
		select {
		case <-timeoutTimer:
			devices := discovery.GetDiscoveredDeviceConfigs()
			return nil, fmt.Errorf("Discovery timed out, expecting %d devices but only %d found", expectedDeviceCount, len(devices))
		case <-tick:
			devices := discovery.GetDiscoveredDeviceConfigs()
			if len(devices) == expectedDeviceCount {
				return devices, nil
			}
		}
	}
}
