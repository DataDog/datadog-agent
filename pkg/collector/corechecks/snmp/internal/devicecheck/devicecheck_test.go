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
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/report"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
)

func TestProfileWithSysObjectIdDetection(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateFakeSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
collect_topology: false
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
 f5-big-ip:
   definition_file: f5-big-ip.yaml
 another-profile:
   definition_file: another_profile.yaml
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

	deviceCk.SetSender(report.NewMetricSender(sender, "", nil))

	(sess.
		SetStr("1.3.6.1.2.1.1.1.0", "my_desc").
		SetObj("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.3375.2.1.3.4.1").
		SetTime("1.3.6.1.2.1.1.3.0", 20).
		SetStr("1.3.6.1.2.1.1.5.0", "foo_sys_name").
		SetInt("1.3.6.1.2.1.2.2.1.13.1", 131).
		SetInt("1.3.6.1.2.1.2.2.1.14.1", 141).
		SetByte("1.3.6.1.2.1.2.2.1.6.1", []byte{00, 00, 00, 00, 00, 01}).
		SetInt("1.3.6.1.2.1.2.2.1.7.1", 1).
		SetInt("1.3.6.1.2.1.2.2.1.8.1", 1).
		SetInt("1.3.6.1.2.1.2.2.1.13.2", 132).
		SetInt("1.3.6.1.2.1.2.2.1.14.2", 142).
		SetByte("1.3.6.1.2.1.2.2.1.6.2", []byte{00, 00, 00, 00, 00, 01}).
		SetInt("1.3.6.1.2.1.2.2.1.7.2", 1).
		SetInt("1.3.6.1.2.1.2.2.1.8.2", 1).
		SetStr("1.3.6.1.2.1.31.1.1.1.1.1", "nameRow1").
		SetStr("1.3.6.1.2.1.31.1.1.1.18.1", "descRow1").
		SetInt("1.3.6.1.2.1.4.20.1.2.10.0.0.1", 1).
		SetIP("1.3.6.1.2.1.4.20.1.3.10.0.0.1", "255.255.255.0").
		SetStr("1.3.6.1.2.1.31.1.1.1.1.2", "nameRow2").
		SetStr("1.3.6.1.2.1.31.1.1.1.18.2", "descRow2").
		SetInt("1.3.6.1.2.1.4.20.1.2.10.0.0.2", 1).
		SetIP("1.3.6.1.2.1.4.20.1.3.10.0.0.2", "255.255.255.0").
		// f5-specific sysStatMemoryTotal
		SetInt("1.3.6.1.4.1.3375.2.1.1.2.1.44.0", 30).
		// Fake metric specific to another_profile
		SetInt("1.3.6.1.2.1.1.999.0", 100))

	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	snmpTags := []string{
		"snmp_device:1.2.3.4",
		"snmp_profile:f5-big-ip",
		"device_vendor:f5",
		"snmp_host:foo_sys_name",
		"static_tag:from_profile_root",
		"some_tag:some_tag_value",
		"prefix:f",
		"suffix:oo_sys_name"}
	telemetryTags := append(common.CopyStrings(snmpTags), "agent_version:"+version.AgentVersion)
	row1Tags := append(common.CopyStrings(snmpTags), "interface:nameRow1", "interface_alias:descRow1", "table_static_tag:val")
	row2Tags := append(common.CopyStrings(snmpTags), "interface:nameRow2", "interface_alias:descRow2", "table_static_tag:val")

	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpTags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(70.5), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInErrors", float64(71), "", row2Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(131), "", row1Tags)
	sender.AssertMetric(t, "MonotonicCount", "snmp.ifInDiscards", float64(132), "", row2Tags)

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", telemetryTags)
	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.submitted_metrics", telemetryTags)

	// Should see f5-specific 'sysStatMemoryTotal' but not fake metrics
	sender.AssertMetric(t, "Gauge", "snmp.sysStatMemoryTotal", float64(60), "", snmpTags)
	sender.AssertNotCalled(t, "Gauge", "snmp.anotherMetric", mock.Anything, mock.Anything, mock.Anything)
	sender.AssertMetricNotTaggedWith(t, "Gauge", "snmp.sysStatMemoryTotal", []string{"unknown_symbol:100"})

	// f5 has 5 metrics, 2 tags
	assert.Len(t, deviceCk.config.Metrics, 5)
	assert.Len(t, deviceCk.config.MetricTags, 2)

	sender.ResetCalls()

	// Switch device sysobjid
	sess.SetObj("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.32473.1.1")
	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	snmpTags = []string{
		"device_namespace:default",
		"snmp_device:1.2.3.4",
		"snmp_profile:another-profile",
		"unknown_symbol:100"}
	telemetryTags = append(common.CopyStrings(snmpTags), "agent_version:"+version.AgentVersion)

	sender.AssertMetric(t, "Gauge", "snmp.sysUpTimeInstance", float64(20), "", snmpTags)

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", telemetryTags)
	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.submitted_metrics", telemetryTags)
	// Should see fake metrics but not f5-specific 'sysStatMemoryTotal'
	sender.AssertMetric(t, "Gauge", "snmp.anotherMetric", float64(100), "", snmpTags)
	sender.AssertNotCalled(t, "Gauge", "snmp.sysStatMemoryTotal", mock.Anything, mock.Anything, mock.Anything)
	sender.AssertMetricNotTaggedWith(t, "Gauge", "snmp.anotherMetric", []string{"some_tag:some_tag_value"})

	// Check that we replaced the metrics, instead of just adding to them
	assert.Len(t, deviceCk.config.Metrics, 2)
	assert.Len(t, deviceCk.config.MetricTags, 2)
}

func TestProfileDetectionPreservesGlobals(t *testing.T) {
	checkconfig.SetConfdPathAndCleanProfiles()
	sess := session.CreateFakeSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
collect_topology: false
use_global_metrics: true
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
 f5-big-ip:
   definition_file: f5-big-ip.yaml
 another_profile:
   definition_file: another_profile.yaml
global_metrics:
  - symbol:
    name: fake.global.metric
    OID: 1.2.3.4.5
`)

	config, err := checkconfig.NewCheckConfig(rawInstanceConfig, rawInitConfig)
	assert.Nil(t, err)

	deviceCk, err := NewDeviceCheck(config, "1.2.3.4", sessionFactory)
	assert.Nil(t, err)

	sender := mocksender.NewMockSender("123") // required to initiate aggregator
	sender.SetupAcceptAll()

	deviceCk.SetSender(report.NewMetricSender(sender, "", nil))

	sess.
		SetObj("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.3375.2.1.3.4.1").
		SetInt("1.2.3.4.5", 12345).
		// f5-specific sysStatMemoryTotal
		SetInt("1.3.6.1.4.1.3375.2.1.1.2.1.44.0", 30).
		// Fake metric specific to another_profile
		SetInt("1.3.6.1.2.1.1.999.0", 100)

	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	sender.AssertMetric(t, "Gauge", "snmp.fake.global.metric", float64(12345), "", []string{"snmp_profile:f5-big-ip"})
	sender.AssertMetric(t, "Gauge", "snmp.sysStatMemoryTotal", float64(60), "", []string{"snmp_profile:f5-big-ip"})
	sender.AssertNotCalled(t, "Gauge", "snmp.anotherMetric", mock.Anything, mock.Anything, mock.Anything)

	// Switch device sysobjid
	sender.ResetCalls()
	sess.SetObj("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.32473.1.1")
	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	sender.AssertMetric(t, "Gauge", "snmp.fake.global.metric", float64(12345), "", []string{"snmp_profile:another_profile"})
	sender.AssertMetric(t, "Gauge", "snmp.anotherMetric", float64(100), "", []string{"snmp_profile:another_profile"})
	sender.AssertNotCalled(t, "Gauge", "snmp.sysStatMemoryTotal", mock.Anything, mock.Anything, mock.Anything)

}

func TestDetectMetricsToCollect(t *testing.T) {
	timeNow = common.MockTimeNow
	defer func() { timeNow = time.Now }()

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "detectmetr.d"))
	config.Datadog.Set("confd_path", profilesWithInvalidExtendConfdPath)

	sess := session.CreateFakeSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
experimental_detect_metrics_enabled: true
experimental_detect_metrics_refresh_interval: 10
collect_topology: false
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

	deviceCk.SetSender(report.NewMetricSender(sender, "", nil))

	sess.
		SetObj("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.3375.2.1.3.4.1").
		SetTime("1.3.6.1.2.1.1.3.0", 20).
		SetStr("1.3.6.1.2.1.1.1.0", "my_desc").
		SetStr("1.3.6.1.2.1.1.5.0", "foo_sys_name").
		SetStr("1.3.6.1.4.1.318.1.1.1.11.1.1.0", "1010").
		SetInt("1.3.6.1.2.1.2.2.1.13.1", 131).
		SetInt("1.3.6.1.2.1.2.2.1.14.1", 141).
		SetStr("1.3.6.1.2.1.2.2.1.2.1", `desc1`).
		SetByte("1.3.6.1.2.1.2.2.1.6.1", []byte{00, 00, 00, 00, 00, 01}).
		SetInt("1.3.6.1.2.1.2.2.1.7.1", 1).
		SetInt("1.3.6.1.2.1.2.2.1.13.2", 132).
		SetInt("1.3.6.1.2.1.2.2.1.14.2", 142).
		SetByte("1.3.6.1.2.1.2.2.1.6.2", []byte{00, 00, 00, 00, 00, 01}).
		SetStr("1.3.6.1.2.1.2.2.1.2.2", `desc2`).
		SetInt("1.3.6.1.2.1.2.2.1.7.2", 1).
		SetInt("1.3.6.1.2.1.2.2.1.8.1", 1).
		SetStr("1.3.6.1.2.1.31.1.1.1.1.1", "nameRow1").
		SetStr("1.3.6.1.2.1.31.1.1.1.18.1", "descRow1").
		SetInt("1.3.6.1.2.1.4.20.1.2.10.0.0.1", 1).
		SetIP("1.3.6.1.2.1.4.20.1.3.10.0.0.1", "255.255.255.0").
		SetInt("1.3.6.1.2.1.2.2.1.8.2", 1).
		SetStr("1.3.6.1.2.1.31.1.1.1.1.2", "nameRow2").
		SetStr("1.3.6.1.2.1.31.1.1.1.18.2", "descRow2").
		SetInt("1.3.6.1.2.1.4.20.1.2.10.0.0.2", 1).
		SetIP("1.3.6.1.2.1.4.20.1.3.10.0.0.2", "255.255.255.0")

	savedAutodetectMetricsTime := deviceCk.nextAutodetectMetrics
	err = deviceCk.Run(timeNow())
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
	sender.AssertNotCalled(t, "Gauge", "snmp.sysStatMemoryTotal", mock.Anything, mock.Anything, mock.Anything)

	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", telemetryTags)
	sender.AssertMetricTaggedWith(t, "MonotonicCount", "datadog.snmp.check_interval", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.check_duration", telemetryTags)
	sender.AssertMetricTaggedWith(t, "Gauge", "datadog.snmp.submitted_metrics", telemetryTags)

	expectedNextAutodetectMetricsTime := savedAutodetectMetricsTime.Add(time.Duration(deviceCk.config.DetectMetricsRefreshInterval) * time.Second)
	assert.WithinDuration(t, expectedNextAutodetectMetricsTime, deviceCk.nextAutodetectMetrics, time.Second)

	expectedMetrics := []profiledefinition.MetricsConfig{
		{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}},
		{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.318.1.1.1.11.1.1.0", Name: "upsBasicStateOutputState"}, MetricType: "flag_stream", Options: profiledefinition.MetricsConfigOption{Placement: 1, MetricSuffix: "OnLine"}},
		{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.318.1.1.1.11.1.1.0", Name: "upsBasicStateOutputState"}, MetricType: "flag_stream", Options: profiledefinition.MetricsConfigOption{Placement: 2, MetricSuffix: "ReplaceBattery"}},
		{
			MetricType: profiledefinition.ProfileMetricTypeMonotonicCount,
			Symbols: []profiledefinition.SymbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors", ScaleFactor: 0.5},
				{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
			},
			MetricTags: []profiledefinition.MetricTagConfig{
				{Tag: "interface", Column: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
				{Tag: "interface_alias", Column: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
				{Tag: "mac_address", Column: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.2.1.6", Name: "ifPhysAddress", Format: "mac_address"}},
			},
			StaticTags: []string{"table_static_tag:val"},
		},
	}

	expectedMetricTags := []profiledefinition.MetricTagConfig{
		{Tag: "snmp_host2", Column: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
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

	assert.ElementsMatch(t, deviceCk.config.Metrics, expectedMetrics)

	assert.ElementsMatch(t, deviceCk.config.MetricTags, expectedMetricTags)

	// Add a new metric and make sure it is added but nothing else is re-added
	sess.SetInt("1.3.6.1.4.1.3375.2.1.1.2.1.44.0", 30)
	sender.ResetCalls()
	timeNow = func() time.Time {
		return common.MockTimeNow().Add(time.Second * 100)
	}

	err = deviceCk.Run(timeNow())
	assert.Nil(t, err)

	sender.AssertMetric(t, "Gauge", "snmp.sysStatMemoryTotal", float64(60), "", snmpTags)

	expectedMetrics = append(expectedMetrics, profiledefinition.MetricsConfig{
		Symbol:     profiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", Name: "sysStatMemoryTotal", ScaleFactor: 2},
		MetricType: profiledefinition.ProfileMetricTypeGauge,
	})
	assert.ElementsMatch(t, expectedMetrics, deviceCk.config.Metrics)
	assert.ElementsMatch(t, expectedMetricTags, deviceCk.config.MetricTags)

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
	deviceCk.SetSender(report.NewMetricSender(sender, "", nil))
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
	deviceCk.SetSender(report.NewMetricSender(sender, "", nil))
	deviceCk.sender.Gauge("snmp.devices_monitored", float64(1), []string{"snmp_device:1.2.3.4"})
	sender.AssertMetric(t, "Gauge", "snmp.devices_monitored", float64(1), "", []string{"snmp_device:1.2.3.4"})

	// with hostname
	deviceCk.SetSender(report.NewMetricSender(sender, "device:123", nil))
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
collect_topology: false
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

	deviceCk.SetSender(report.NewMetricSender(sender, "", nil))

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
	sess.On("GetBulk", []string{"1.3.6.1.2.1.31.1.1.1.1", "1.3.6.1.2.1.31.1.1.1.18", "1.3.6.1.2.1.4.20.1.2", "1.3.6.1.2.1.4.20.1.3"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPackets[1], nil)

	err = deviceCk.Run(time.Now())
	assert.Nil(t, err)

	snmpTags := []string{"snmp_device:1.2.3.4", "snmp_profile:f5-big-ip", "device_vendor:f5", "snmp_host:foo_sys_name",
		"static_tag:from_profile_root", "some_tag:some_tag_value", "prefix:f", "suffix:oo_sys_name"}

	sender.AssertServiceCheck(t, "snmp.can_check", servicecheck.ServiceCheckOK, "", snmpTags, "")
	sender.AssertMetric(t, "Gauge", deviceReachableMetric, 1., "", snmpTags)
	sender.AssertMetric(t, "Gauge", deviceUnreachableMetric, 0., "", snmpTags)

	sender.ResetCalls()
	sess.ConnectErr = fmt.Errorf("some error")
	err = deviceCk.Run(time.Now())

	assert.Error(t, err, "some error")
	sender.Mock.AssertCalled(t, "ServiceCheck", "snmp.can_check", servicecheck.ServiceCheckCritical, "", mocksender.MatchTagsContains(snmpTags), "snmp connection error: some error")
	sender.AssertMetric(t, "Gauge", deviceUnreachableMetric, 1., "", snmpTags)
	sender.AssertMetric(t, "Gauge", deviceReachableMetric, 0., "", snmpTags)
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

	deviceCk.SetSender(report.NewMetricSender(sender, "", nil))

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

	deviceCk.SetSender(report.NewMetricSender(sender, "", nil))

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.2.0"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.1.5.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.1.5.0"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.13.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.14"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.14.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.15"}).Return(session.CreateGetNextPacket("1.3.6.1.2.1.2.2.1.15.1", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.2.1.2.2.1.16"}).Return(session.CreateGetNextPacket("1.3.6.1.4.1.3375.2.1.1.2.1.44.0", gosnmp.OctetString, []byte(`123`)), nil)
	sess.On("GetNext", []string{"1.3.6.1.4.1.3375.2.1.1.2.1.44.0"}).Return(session.CreateGetNextPacket("", gosnmp.EndOfMibView, nil), nil)

	metricsConfigs, metricTagConfigs := deviceCk.detectAvailableMetrics()

	expectedMetricsConfigs := []profiledefinition.MetricsConfig{
		{
			Symbol:     profiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", Name: "sysStatMemoryTotal", ScaleFactor: 2},
			MetricType: profiledefinition.ProfileMetricTypeGauge,
		},
		{
			MetricType: profiledefinition.ProfileMetricTypeMonotonicCount,
			Symbols: []profiledefinition.SymbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors", ScaleFactor: 0.5},
				{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
			},
			MetricTags: []profiledefinition.MetricTagConfig{
				{Tag: "interface", Column: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
				{Tag: "interface_alias", Column: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
				{Tag: "mac_address", Column: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.2.1.6", Name: "ifPhysAddress", Format: "mac_address"}},
			},
			StaticTags: []string{"table_static_tag:val"},
		},
	}
	assert.ElementsMatch(t, expectedMetricsConfigs, metricsConfigs)

	expectedMetricsTagConfigs := []profiledefinition.MetricTagConfig{
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
		{Tag: "snmp_host2", Column: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
	}

	checkconfig.ValidateEnrichMetricTags(expectedMetricsTagConfigs)

	assert.ElementsMatch(t, expectedMetricsTagConfigs, metricTagConfigs)
}
