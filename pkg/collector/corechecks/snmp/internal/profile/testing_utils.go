// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package profile

import (
	"bufio"
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// CopyProfileDefinition copies a profile, it's used for testing
func CopyProfileDefinition(profileDef profiledefinition.ProfileDefinition) profiledefinition.ProfileDefinition {
	return *profileDef.Clone()
}

// SetConfdPathAndCleanProfiles is used for testing only
func SetConfdPathAndCleanProfiles() {
	SetGlobalProfileConfigMap(nil) // make sure from the new confd path will be reloaded
	file, _ := filepath.Abs(filepath.Join(".", "test", "conf.d"))
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join("..", "test", "conf.d"))
	}
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join(".", "internal", "test", "conf.d"))
	}
	pkgconfigsetup.Datadog().SetInTest("confd_path", file)
}

// FixtureProfileDefinitionMap returns a fixture of ProfileConfigMap with `f5-big-ip` profile
func FixtureProfileDefinitionMap() ProfileConfigMap {
	metrics := []profiledefinition.MetricsConfig{
		{MIB: "F5-BIGIP-SYSTEM-MIB", Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", Name: "sysStatMemoryTotal", ScaleFactor: 2}, MetricType: profiledefinition.ProfileMetricTypeGauge},
		{MIB: "F5-BIGIP-SYSTEM-MIB", Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.999", Name: "oldSyntax"}},
		{
			MIB: "IF-MIB",
			Table: profiledefinition.SymbolConfig{
				OID:  "1.3.6.1.2.1.2.2",
				Name: "ifTable",
			},
			MetricType: profiledefinition.ProfileMetricTypeMonotonicCount,
			Symbols: []profiledefinition.SymbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors", ScaleFactor: 0.5},
				{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
			},
			MetricTags: []profiledefinition.MetricTagConfig{
				{Tag: "interface", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
				{Tag: "interface_alias", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
				{Tag: "mac_address", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.2.2.1.6", Name: "ifPhysAddress", Format: "mac_address"}},
			},
			StaticTags: []string{"table_static_tag:val"},
		},
		{MIB: "SOME-MIB", Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
	}
	return ProfileConfigMap{
		"f5-big-ip": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Name:         "f5-big-ip",
				Metrics:      metrics,
				Extends:      []string{"_base.yaml", "_generic-if.yaml"},
				Device:       profiledefinition.DeviceMeta{Vendor: "f5"},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3.4.*"},
				StaticTags:   []string{"static_tag:from_profile_root", "static_tag:from_base_profile"},
				MetricTags: []profiledefinition.MetricTagConfig{
					{
						Symbol:  profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
						Match:   "(\\w)(\\w+)",
						Pattern: regexp.MustCompile(`(\w)(\w+)`),
						Tags: map[string]string{
							"some_tag": "some_tag_value",
							"prefix":   "\\1",
							"suffix":   "\\2",
						},
					},
					{Tag: "snmp_host", Index: 0x0, Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
				},
				Metadata: profiledefinition.MetadataConfig{
					"device": {
						Fields: map[string]profiledefinition.MetadataField{
							"vendor": {
								Value: "f5",
							},
							"description": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
							"name": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
							"serial_number": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.4.1.3375.2.1.3.3.3.0",
									Name: "sysGeneralChassisSerialNum",
								},
							},
							"sys_object_id": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.2.0",
									Name: "sysObjectID",
								},
							},
							"type": {
								Value: "load_balancer",
							},
						},
					},
					"interface": {
						Fields: map[string]profiledefinition.MetadataField{
							"admin_status": {
								Symbol: profiledefinition.SymbolConfig{

									OID:  "1.3.6.1.2.1.2.2.1.7",
									Name: "ifAdminStatus",
								},
							},
							"alias": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1.1.18",
									Name: "ifAlias",
								},
							},
							"description": {
								Symbol: profiledefinition.SymbolConfig{
									OID:                  "1.3.6.1.2.1.31.1.1.1.1",
									Name:                 "ifName",
									ExtractValue:         "(Row\\d)",
									ExtractValueCompiled: regexp.MustCompile(`(Row\d)`),
								},
							},
							"mac_address": {
								Symbol: profiledefinition.SymbolConfig{
									OID:    "1.3.6.1.2.1.2.2.1.6",
									Name:   "ifPhysAddress",
									Format: "mac_address",
								},
							},
							"name": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
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
							{
								Tag: "custom-tag",
								Symbol: profiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifAlias",
								},
							},
							{
								Tag: "interface",
								Symbol: profiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
						},
					},
				},
			},
			IsUserProfile: true,
		},
		"another_profile": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Name:         "another_profile",
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.32473.1.1"},
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.999.0", Name: "anotherMetric"}, MetricType: ""},
				},
				MetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "snmp_host2", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
					{Tag: "unknown_symbol", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.999.0", Name: "unknownSymbol"}},
				},
				Metadata: profiledefinition.MetadataConfig{},
			},
			IsUserProfile: true,
		},
	}
}

// LogValidator provides assertion helpers against captured log messages.
type LogValidator struct {
	b *bytes.Buffer
	w *bufio.Writer
	l log.LoggerInterface
}

// TrapLogs creates a LogValidator and sets it as the agent-wide logger.
func TrapLogs(t testing.TB, level log.LogLevel) LogValidator {
	t.Helper()
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, level)
	if err != nil {
		t.Errorf("Failed to create a logger: %v", err)
		return LogValidator{}
	}
	log.SetupLogger(l, level.String())
	return LogValidator{b: &b, w: w, l: l}
}

// LogCount represents a log message and the expected number of times it should occur.
type LogCount struct {
	log   string
	count int
}

// GetLogs flushes the logs writer and returns all logs captured so far as a string.
func (lv *LogValidator) GetLogs() (string, error) {
	err := lv.w.Flush()
	if err != nil {
		return "", err
	}
	return lv.b.String(), nil
}

// AssertAbsent t.Error()s if any of the given strings appears in the logs.
// It will print out the full logs only if it finds an error.
func (lv *LogValidator) AssertAbsent(t testing.TB, unexpected ...string) bool {
	t.Helper()
	logs, err := lv.GetLogs()
	if err != nil {
		t.Errorf("Failed to get logs: %v", err)
		return false
	}
	ok := true
	for _, line := range unexpected {
		if strings.Contains(logs, line) {
			ok = false
			t.Errorf("Unexpected log message: %q", line)
		}
	}
	if !ok {
		t.Log("Full logs:\n", logs)
	}
	return ok
}

// AssertPresent t.Error()s if any expected string never appeared in the logs.
// It will print out the full logs only if at least one string is missing.
func (lv *LogValidator) AssertPresent(t testing.TB, expected ...string) bool {
	t.Helper()
	logs, err := lv.GetLogs()
	if err != nil {
		t.Errorf("Failed to get logs: %v", err)
		return false
	}
	ok := true
	for _, line := range expected {
		if !strings.Contains(logs, line) {
			ok = false
			t.Errorf("Missing log message: %q", line)
		}
	}
	if !ok {
		t.Log("Full logs:\n", logs)
	}
	return ok
}

// AssertContains asserts that each expected line in LogCount appears the
// expected number of times, printing useful messages if not. This will only
// print out the full logs if any of the assertions fail, and will only print
// the full logs once even if many assertions fail.
func (lv *LogValidator) AssertContains(t testing.TB, expected []LogCount) bool {
	t.Helper()
	logs, err := lv.GetLogs()
	if err != nil {
		t.Errorf("Failed to get logs: %v", err)
		return false
	}
	ok := true
	for _, aLogCount := range expected {
		n := strings.Count(logs, aLogCount.log)
		if n == 0 && aLogCount.count > 0 {
			ok = false
			t.Errorf("Missing log message: %q", aLogCount.log)
		} else if aLogCount.count == 0 && n != 0 {
			ok = false
			t.Errorf("Unexpected log message: %q", aLogCount.log)
		} else if aLogCount.count != n {
			ok = false
			t.Errorf("Unexpected log message count (expected %d, got %d): %q", aLogCount.count, n, aLogCount.log)
		}
	}
	if !ok {
		t.Log("Full logs:\n", logs)
	}
	return ok
}
