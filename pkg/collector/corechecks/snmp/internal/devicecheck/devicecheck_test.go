// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package devicecheck

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/report"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

func TestProfileWithSysObjectIdDetection(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
 f5-big-ip:
   definition_file: f5-big-ip.yaml
`)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	deviceCk, err := NewDeviceCheck(config, "1.2.3.4", sessionFactory)
	assert.Nil(t, err)

	sender := mocksender.NewMockSender("123") // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	deviceCk.SetSender(report.NewMetricSender(sender, ""))

	sysObjectIDPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}

	packets := []gosnmp.SnmpPacket{
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.2.3.4.5",
					Type:  gosnmp.NoSuchObject,
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
					Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
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
			},
		},
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.2.3.4.5",
					Type:  gosnmp.NoSuchObject,
					Value: nil,
				},
			},
		},
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
					Type:  gosnmp.Integer,
					Value: 30,
				},
				{
					Name:  "1.3.6.1.4.1.3375.2.1.1.2.1.44.999.0",
					Type:  gosnmp.NoSuchObject,
					Value: nil,
				},
			},
		},
	}

	bulkPackets := []gosnmp.SnmpPacket{
		{
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
					Value: []byte{00, 00, 00, 00, 00, 01},
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
					Value: []byte{00, 00, 00, 00, 00, 01},
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
				}},
		},
		{
			Variables: []gosnmp.SnmpPDU{

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
			},
		},
	}

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&sysObjectIDPacket, nil)
	sess.On("Get", []string{
		"1.2.3.4.5",
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.1.5.0",
	}).Return(&packets[0], nil)
	sess.On("Get", []string{
		"1.2.3.4.5.0",
	}).Return(&packets[1], nil)
	sess.On("Get", []string{
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.999",
		"1.3.6.1.4.1.3375.2.1.3.3.3.0",
	}).Return(&packets[2], nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.13", "1.3.6.1.2.1.2.2.1.14", "1.3.6.1.2.1.2.2.1.6", "1.3.6.1.2.1.2.2.1.7", "1.3.6.1.2.1.2.2.1.8"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPackets[0], nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.31.1.1.1.1", "1.3.6.1.2.1.31.1.1.1.18"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPackets[1], nil)

	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	snmpTags := []string{"snmp_device:1.2.3.4", "snmp_profile:f5-big-ip", "device_vendor:f5", "snmp_host:foo_sys_name",
		"static_tag:from_profile_root", "some_tag:some_tag_value", "prefix:f", "suffix:oo_sys_name"}
	telemetryTags := append(common.CopyStrings(snmpTags), "agent_version:"+version.AgentVersion)
	row1Tags := append(common.CopyStrings(snmpTags), "interface:nameRow1", "interface_alias:descRow1", "table_static_tag:val")
	row2Tags := append(common.CopyStrings(snmpTags), "interface:nameRow2", "interface_alias:descRow2", "table_static_tag:val")

	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpTags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(70.5), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(71), "", row2Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(131), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(132), "", row2Tags)
	sender.AssertMetric(t, "Gauge", "snmp.sysStatMemoryTotal", float64(60), "", snmpTags)

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", telemetryTags)
	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.submitted_metrics", telemetryTags)

	assert.Equal(t, false, deviceCk.config.AutodetectProfile)

	// Make sure we don't auto detect and add metrics twice if we already did that previously
	firstRunMetrics := deviceCk.config.Metrics
	firstRunMetricsTags := deviceCk.config.MetricTags
	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	assert.Len(t, deviceCk.config.Metrics, len(firstRunMetrics))
	assert.Len(t, deviceCk.config.MetricTags, len(firstRunMetricsTags))
}

func TestDetectMetricsToCollect(t *testing.T) {
	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "detectmetr.d"))
	config.Datadog.Set("confd_path", profilesWithInvalidExtendConfdPath)

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
experimental_detect_metrics_enabled: true
`)
	// language=yaml
	rawInitConfig := []byte(``)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	deviceCk, err := NewDeviceCheck(config, "1.2.3.4", sessionFactory)
	assert.Nil(t, err)

	sender := mocksender.NewMockSender("123") // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	deviceCk.SetSender(report.NewMetricSender(sender, ""))

	sysObjectIDPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}

	packets := []gosnmp.SnmpPacket{
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.3.6.1.2.1.1.1.0",
					Type:  gosnmp.OctetString,
					Value: []byte("my_desc"),
				},
				{
					Name:  "1.3.6.1.2.1.1.2.0",
					Type:  gosnmp.ObjectIdentifier,
					Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
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
					Name:  "1.3.6.1.4.1.318.1.1.1.11.1.1.0",
					Type:  gosnmp.OctetString,
					Value: []byte("1010"),
				},
			},
		},
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
					Type:  gosnmp.Integer,
					Value: 30,
				},
			},
		},
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.2.3.4.5",
					Type:  gosnmp.NoSuchObject,
					Value: nil,
				},
			},
		},
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.3.6.1.4.1.3375.2.1.1.2.1.44.999.0",
					Type:  gosnmp.NoSuchObject,
					Value: nil,
				},
			},
		},
	}

	bulkPackets := []gosnmp.SnmpPacket{
		{
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
					Value: []byte(`desc1`),
				},
				{
					Name:  "1.3.6.1.2.1.2.2.1.6.1",
					Type:  gosnmp.OctetString,
					Value: []byte{00, 00, 00, 00, 00, 01},
				},
				{
					Name:  "1.3.6.1.2.1.2.2.1.7.1",
					Type:  gosnmp.Integer,
					Value: 1,
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
					Value: []byte{00, 00, 00, 00, 00, 01},
				},
				{
					Name:  "1.3.6.1.2.1.2.2.1.2.2",
					Type:  gosnmp.OctetString,
					Value: []byte(`desc2`),
				},
				{
					Name:  "1.3.6.1.2.1.2.2.1.7.2",
					Type:  gosnmp.Integer,
					Value: 1,
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
				}},
		},
		{
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
			},
		},
	}

	//sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("GetNext", []string{"1.0"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.1.1.0", gosnmp.OctetString, []byte(`mydesc`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.2.0"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.1.3.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.3.0"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.1.5.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.5.0"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.6.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.7"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.7.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.8"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.8.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.9"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.13.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.14"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.14.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.15"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.31.1.1.1.1.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.31.1.1.1.1.2"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.31.1.1.1.18.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.31.1.1.1.19"}).Return(session.CreateGetNextPacket("1.3.6.1.4.1.318.1.1.1.11.1.1.0", gosnmp.OctetString, []byte(`1010`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.4.1.318.1.1.1.11.1.1.0"}).Return(session.CreateGetNextPacket("1.3.6.1.4.1.3375.2.1.1.2.1.44.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.4.1.3375.2.1.1.2.1.44.0"}).Return(session.CreateGetNextPacket("1.3.6.1.4.1.3375.2.1.1.2.1.44.999.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.4.1.3375.2.1.1.2.1.44.999.0"}).Return(session.CreateGetNextPacket("1.3.6.1.4.1.3375.2.1.3.3.3.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.4.1.3375.2.1.3.3.3.0"}).Return(session.CreateGetNextPacket("1.3.6.1.4.1.3375.2.1.3.3.3.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.4.1.3375.2.1.3.3.4.0"}).Return(session.CreateGetNextPacket("1.3.6.1.4.1.3375.2.1.3.3.4.0", gosnmp.EndOfMibView, []byte(`123`)), nil)
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&sysObjectIDPacket, nil)
	sess.On("Get", []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.1.5.0",
		"1.3.6.1.4.1.318.1.1.1.11.1.1.0",
	}).Return(&packets[0], nil)
	sess.On("Get", []string{
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
	}).Return(&packets[1], nil)
	sess.On("Get", []string{
		"1.2.3.4.5.0",
	}).Return(&packets[2], nil)
	sess.On("Get", []string{
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.999",
	}).Return(&packets[3], nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.13", "1.3.6.1.2.1.2.2.1.14", "1.3.6.1.2.1.2.2.1.2", "1.3.6.1.2.1.2.2.1.6", "1.3.6.1.2.1.2.2.1.7"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPackets[0], nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.8", "1.3.6.1.2.1.31.1.1.1.1", "1.3.6.1.2.1.31.1.1.1.18"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPackets[1], nil)

	savedAutodetectMetricsTime := deviceCk.nextAutodetectMetrics
	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	snmpTags := []string{"snmp_device:1.2.3.4"}
	telemetryTags := append(common.CopyStrings(snmpTags), "agent_version:"+version.AgentVersion)
	row1Tags := append(common.CopyStrings(snmpTags), "interface:nameRow1", "interface_alias:descRow1", "table_static_tag:val")
	row2Tags := append(common.CopyStrings(snmpTags), "interface:nameRow2", "interface_alias:descRow2", "table_static_tag:val")

	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpTags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(70.5), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(71), "", row2Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(131), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(132), "", row2Tags)
	sender.AssertMetric(t, "Gauge", "snmp.sysStatMemoryTotal", float64(60), "", snmpTags)

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", telemetryTags)
	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.submitted_metrics", telemetryTags)

	expectedNextAutodetectMetricsTime := savedAutodetectMetricsTime.Add(time.Duration(deviceCk.config.DetectMetricsRefreshInterval) * time.Second)
	assert.WithinDuration(t, expectedNextAutodetectMetricsTime, deviceCk.nextAutodetectMetrics, 3*time.Second)

	// Make sure we don't auto detect and add metrics twice if we already did that previously
	firstRunMetrics := deviceCk.config.Metrics
	firstRunMetricsTags := deviceCk.config.MetricTags
	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	expectedMetrics := []checkconfig.MetricsConfig{
		{Symbol: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
		{Symbol: checkconfig.SymbolConfig{OID: "1.3.6.1.4.1.318.1.1.1.11.1.1.0", Name: "upsBasicStateOutputState"}, ForcedType: "flag_stream", Options: checkconfig.MetricsConfigOption{Placement: 1, MetricSuffix: "OnLine"}},
		{Symbol: checkconfig.SymbolConfig{OID: "1.3.6.1.4.1.318.1.1.1.11.1.1.0", Name: "upsBasicStateOutputState"}, ForcedType: "flag_stream", Options: checkconfig.MetricsConfigOption{Placement: 2, MetricSuffix: "ReplaceBattery"}},
		{
			ForcedType: "monotonic_count",
			Symbols: []checkconfig.SymbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors", ScaleFactor: 0.5},
				{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
			},
			MetricTags: []checkconfig.MetricTagConfig{
				{Tag: "interface", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
				{Tag: "interface_alias", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
				{Tag: "mac_address", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.2.2.1.6", Name: "ifPhysAddress", Format: "mac_address"}},
			},
			StaticTags: []string{"table_static_tag:val"},
		},
		{Symbol: checkconfig.SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", Name: "sysStatMemoryTotal", ScaleFactor: 2}, ForcedType: "gauge"},
	}

	expectedMetricTags := []checkconfig.MetricTagConfig{
		{Tag: "snmp_host2", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
		{
			OID:   "1.3.6.1.2.1.1.5.0",
			Name:  "sysName",
			Match: "(\\w)(\\w+)",
			Tags: map[string]string{
				"prefix":   "\\1",
				"suffix":   "\\2",
				"some_tag": "some_tag_value",
			},
		},
		{Tag: "snmp_host", OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
	}
	checkconfig.ValidateEnrichMetrics(expectedMetrics)
	checkconfig.ValidateEnrichMetricTags(expectedMetricTags)

	fmt.Printf("Actual Metrics   : %+v\n", deviceCk.config.Metrics)
	fmt.Printf("Expected Metrics : %+v\n", expectedMetrics)
	assert.ElementsMatch(t, deviceCk.config.Metrics, expectedMetrics)

	fmt.Printf("Actual MetricTags   : %+v\n", deviceCk.config.MetricTags)
	fmt.Printf("Expected MetricTags : %+v\n", expectedMetricTags)
	assert.ElementsMatch(t, deviceCk.config.MetricTags, expectedMetricTags)

	assert.Equal(t, 5, len(deviceCk.config.Metrics))
	assert.Equal(t, 3, len(deviceCk.config.MetricTags))
	assert.Len(t, deviceCk.config.Metrics, len(firstRunMetrics))
	assert.Len(t, deviceCk.config.MetricTags, len(firstRunMetricsTags))
}

func TestDetectMetricsToCollect_detectMetricsToMonitor_nextAutodetectMetrics(t *testing.T) {
	timeNow = common.MockTimeNow
	defer func() { timeNow = time.Now }()

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
experimental_detect_metrics_enabled: true
experimental_detect_metrics_refresh_interval: 600 # 10min
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
 f5-big-ip:
   definition_file: f5-big-ip.yaml
`)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	deviceCk, err := NewDeviceCheck(config, "1.2.3.4", sessionFactory)
	assert.Nil(t, err)

	sender := mocksender.NewMockSender("123") // required to initiate aggregator
	deviceCk.SetSender(report.NewMetricSender(sender, ""))
	sess.On("GetNext", []string{"1.0"}).Return(session.CreateGetNextPacket("9999", gosnmp.EndOfMibView, nil), nil)

	deviceCk.detectMetricsToMonitor(sess)

	expectedNextAutodetectMetricsTime := common.MockTimeNow().Add(600 * time.Second)
	assert.Equal(t, expectedNextAutodetectMetricsTime, deviceCk.nextAutodetectMetrics)

	// 10 seconds after
	timeNow = func() time.Time {
		return common.MockTimeNow().Add(10 * time.Second)
	}
	deviceCk.detectMetricsToMonitor(sess)
	assert.Equal(t, expectedNextAutodetectMetricsTime, deviceCk.nextAutodetectMetrics)

	// 599 seconds after
	timeNow = func() time.Time {
		return common.MockTimeNow().Add(599 * time.Second)
	}
	deviceCk.detectMetricsToMonitor(sess)
	assert.Equal(t, expectedNextAutodetectMetricsTime, deviceCk.nextAutodetectMetrics)

	// 600 seconds after
	expectedNextAutodetectMetricsTime = common.MockTimeNow().Add(1200 * time.Second)
	timeNow = func() time.Time {
		return common.MockTimeNow().Add(600 * time.Second)
	}
	deviceCk.detectMetricsToMonitor(sess)
	assert.Equal(t, expectedNextAutodetectMetricsTime, deviceCk.nextAutodetectMetrics)
}

func TestDeviceCheck_Hostname(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
`)
	// language=yaml
	rawInitConfig := []byte(`
`)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	deviceCk, err := NewDeviceCheck(config, "1.2.3.4", session.NewMockSession)
	assert.Nil(t, err)

	sender := mocksender.NewMockSender("123") // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	// without hostname
	deviceCk.SetSender(report.NewMetricSender(sender, ""))
	deviceCk.sender.Gauge("snmp.devices_monitored", float64(1), []string{"snmp_device:1.2.3.4"})
	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", []string{"snmp_device:1.2.3.4"})

	// with hostname
	deviceCk.SetSender(report.NewMetricSender(sender, "device:123"))
	deviceCk.sender.Gauge("snmp.devices_monitored", float64(1), []string{"snmp_device:1.2.3.4"})
	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "device:123", []string{"snmp_device:1.2.3.4"})
}

func TestDeviceCheck_GetHostname(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
`)
	// language=yaml
	rawInitConfig := []byte(`
`)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	deviceCk, err := NewDeviceCheck(config, "1.2.3.4", session.NewMockSession)
	assert.Nil(t, err)

	hostname, err := deviceCk.GetDeviceHostname()
	assert.Nil(t, err)
	assert.Equal(t, "", hostname)

	deviceCk.config.UseDeviceIDAsHostname = true
	hostname, err = deviceCk.GetDeviceHostname()
	assert.Nil(t, err)
	assert.Equal(t, "device:default:1.2.3.4", hostname)

	deviceCk.config.UseDeviceIDAsHostname = false
	hostname, err = deviceCk.GetDeviceHostname()
	assert.Nil(t, err)
	assert.Equal(t, "", hostname)

	deviceCk.config.UseDeviceIDAsHostname = true
	deviceCk.config.Namespace = "a>b"
	deviceCk.config.UpdateDeviceIDAndTags()
	hostname, err = deviceCk.GetDeviceHostname()
	assert.Nil(t, err)
	assert.Equal(t, "device:a-b:1.2.3.4", hostname)

	deviceCk.config.Namespace = "a<b"
	deviceCk.config.UpdateDeviceIDAndTags()
	hostname, err = deviceCk.GetDeviceHostname()
	assert.Nil(t, err)
	assert.Equal(t, "device:a-b:1.2.3.4", hostname)

	deviceCk.config.Namespace = "a\n\r\tb"
	deviceCk.config.UpdateDeviceIDAndTags()
	hostname, err = deviceCk.GetDeviceHostname()
	assert.Nil(t, err)
	assert.Equal(t, "device:ab:1.2.3.4", hostname)

	deviceCk.config.Namespace = strings.Repeat("a", 256)
	deviceCk.config.UpdateDeviceIDAndTags()
	hostname, err = deviceCk.GetDeviceHostname()
	assert.NotNil(t, err)
	assert.Equal(t, "", hostname)
}

func TestDynamicTagsAreSaved(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
 f5-big-ip:
   definition_file: f5-big-ip.yaml
`)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	deviceCk, err := NewDeviceCheck(config, "1.2.3.4", sessionFactory)
	assert.Nil(t, err)

	sender := mocksender.NewMockSender("123") // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	deviceCk.SetSender(report.NewMetricSender(sender, ""))

	sysObjectIDPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}

	packets := []gosnmp.SnmpPacket{
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.2.3.4.5",
					Type:  gosnmp.NoSuchObject,
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
					Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
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
			},
		},
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.2.3.4.5",
					Type:  gosnmp.NoSuchObject,
					Value: nil,
				},
			},
		},
		{
			Variables: []gosnmp.SnmpPDU{
				{
					Name:  "1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
					Type:  gosnmp.Integer,
					Value: 30,
				},
				{
					Name:  "1.3.6.1.4.1.3375.2.1.1.2.1.44.999.0",
					Type:  gosnmp.NoSuchObject,
					Value: nil,
				},
			},
		},
	}

	bulkPackets := []gosnmp.SnmpPacket{
		{
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
					Value: []byte{00, 00, 00, 00, 00, 01},
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
					Value: []byte{00, 00, 00, 00, 00, 01},
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
				}},
		},
		{
			Variables: []gosnmp.SnmpPDU{

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
			},
		},
	}

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&sysObjectIDPacket, nil)
	sess.On("Get", []string{
		"1.2.3.4.5",
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.1.5.0",
	}).Return(&packets[0], nil)
	sess.On("Get", []string{
		"1.2.3.4.5.0",
	}).Return(&packets[1], nil)
	sess.On("Get", []string{
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.0",
		"1.3.6.1.4.1.3375.2.1.1.2.1.44.999",
		"1.3.6.1.4.1.3375.2.1.3.3.3.0",
	}).Return(&packets[2], nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.2.2.1.13", "1.3.6.1.2.1.2.2.1.14", "1.3.6.1.2.1.2.2.1.6", "1.3.6.1.2.1.2.2.1.7", "1.3.6.1.2.1.2.2.1.8"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPackets[0], nil)
	sess.On("GetBulk", []string{"1.3.6.1.2.1.31.1.1.1.1", "1.3.6.1.2.1.31.1.1.1.18"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPackets[1], nil)

	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	snmpTags := []string{"snmp_device:1.2.3.4", "snmp_profile:f5-big-ip", "device_vendor:f5", "snmp_host:foo_sys_name",
		"static_tag:from_profile_root", "some_tag:some_tag_value", "prefix:f", "suffix:oo_sys_name"}

	sender.AssertServiceCheck(t, "snmp.can_check", metrics.ServiceCheckOK, "", snmpTags, "")
	assert.Equal(t, false, deviceCk.config.AutodetectProfile)

	sess.ConnectErr = fmt.Errorf("some error")
	err = deviceCk.Run(time.Now())

	assert.Error(t, err, "some error")
	sender.Mock.AssertCalled(t, "ServiceCheck", "snmp.can_check", metrics.ServiceCheckCritical, "", mocksender.MatchTagsContains(snmpTags), "snmp connection error: some error")
}

func TestRun_sessionCloseError(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateMockSession()
	sess.CloseErr = fmt.Errorf("close error")
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

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
	// language=yaml
	rawInitConfig := []byte(`
profiles:
 f5-big-ip:
   definition_file: f5-big-ip.yaml
`)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	deviceCk, err := NewDeviceCheck(config, "1.2.3.4", sessionFactory)
	assert.Nil(t, err)

	sender := mocksender.NewMockSender("123") // required to initiate aggregator
	sender.SetupAcceptAll()

	deviceCk.SetSender(report.NewMetricSender(sender, ""))

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{},
	}
	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", []string{"1.2.3", "1.3.6.1.2.1.1.3.0"}).Return(&packet, nil)

	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	assert.Equal(t, uint64(1), deviceCk.sessionCloseErrorCount.Load())
}

func TestDeviceCheck_detectAvailableMetrics(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	// language=yaml
	rawInstanceConfig := []byte(`
collect_device_metadata: false
ip_address: 1.2.3.4
community_string: public
`)
	// language=yaml
	rawInitConfig := []byte(``)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	deviceCk, err := NewDeviceCheck(config, "1.2.3.4", sessionFactory)
	assert.Nil(t, err)

	sender := mocksender.NewMockSender("123") // required to initiate aggregator
	sender.SetupAcceptAll()

	deviceCk.SetSender(report.NewMetricSender(sender, ""))

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.2.0"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.1.5.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.5.0"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.13.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.14"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.14.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.15"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.15.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.16"}).Return(session.CreateGetNextPacket("1.3.6.1.4.1.3375.2.1.1.2.1.44.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.4.1.3375.2.1.1.2.1.44.0"}).Return(session.CreateGetNextPacket("", gosnmp.EndOfMibView, nil), nil)

	metricsConfigs, metricTagConfigs := deviceCk.detectAvailableMetrics()

	expectedMetricsConfigs := []checkconfig.MetricsConfig{
		{
			Symbol:     checkconfig.SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", Name: "sysStatMemoryTotal", ScaleFactor: 2},
			ForcedType: "gauge",
		},
		{
			ForcedType: "monotonic_count",
			Symbols: []checkconfig.SymbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors", ScaleFactor: 0.5},
				{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
			},
			MetricTags: []checkconfig.MetricTagConfig{
				{Tag: "interface", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
				{Tag: "interface_alias", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
				{Tag: "mac_address", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.2.2.1.6", Name: "ifPhysAddress", Format: "mac_address"}},
			},
			StaticTags: []string{"table_static_tag:val"},
		},
	}
	assert.ElementsMatch(t, expectedMetricsConfigs, metricsConfigs)

	expectedMetricsTagConfigs := []checkconfig.MetricTagConfig{
		{
			OID:   "1.3.6.1.2.1.1.5.0",
			Name:  "sysName",
			Match: "(\\w)(\\w+)",
			Tags: map[string]string{
				"some_tag": "some_tag_value",
				"prefix":   "\\1",
				"suffix":   "\\2",
			},
		},
		{Tag: "snmp_host", OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
		{Tag: "snmp_host2", Column: checkconfig.SymbolConfig{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
	}

	checkconfig.ValidateEnrichMetricTags(expectedMetricsTagConfigs)

	assert.ElementsMatch(t, expectedMetricsTagConfigs, metricTagConfigs)
}
