// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	assert "github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func fixtureProfileDefinitionMap() profileDefinitionMap {
	metrics := []MetricsConfig{
		{Symbol: SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", Name: "sysStatMemoryTotal", ScaleFactor: 2}, ForcedType: "gauge"},
		{Symbol: SymbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.999", Name: "oldSyntax"}},
		{
			ForcedType: "monotonic_count",
			Symbols: []SymbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors", ScaleFactor: 0.5},
				{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
			},
			MetricTags: []MetricTagConfig{
				{Tag: "interface", Column: SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
				{Tag: "interface_alias", Column: SymbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
				{Tag: "mac_address", Column: SymbolConfig{OID: "1.3.6.1.2.1.2.2.1.6", Name: "ifPhysAddress", Format: "mac_address"}},
			},
			StaticTags: []string{"table_static_tag:val"},
		},
		{Symbol: SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
	}
	return profileDefinitionMap{
		"f5-big-ip": profileDefinition{
			Metrics:      metrics,
			Extends:      []string{"_base.yaml", "_generic-if.yaml"},
			Device:       DeviceMeta{Vendor: "f5"},
			SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.*"},
			StaticTags:   []string{"static_tag:from_profile_root", "static_tag:from_base_profile"},
			MetricTags: []MetricTagConfig{
				{
					OID:     "1.3.6.1.2.1.1.5.0",
					Name:    "sysName",
					Match:   "(\\w)(\\w+)",
					pattern: regexp.MustCompile("(\\w)(\\w+)"),
					Tags: map[string]string{
						"some_tag": "some_tag_value",
						"prefix":   "\\1",
						"suffix":   "\\2",
					},
				},
				{Tag: "snmp_host", Index: 0x0, Column: SymbolConfig{OID: "", Name: ""}, OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
			},
			Metadata: MetadataConfig{
				"device": {
					Fields: map[string]MetadataField{
						"vendor": {
							Value: "f5",
						},
						"description": {
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.2.1.1.1.0",
								Name: "sysDescr",
							},
						},
						"name": {
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.2.1.1.5.0",
								Name: "sysName",
							},
						},
						"serial_number": {
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.4.1.3375.2.1.3.3.3.0",
								Name: "sysGeneralChassisSerialNum",
							},
						},
						"sys_object_id": {
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.2.1.1.2.0",
								Name: "sysObjectID",
							},
						},
					},
				},
				"interface": {
					Fields: map[string]MetadataField{
						"admin_status": {
							Symbol: SymbolConfig{

								OID:  "1.3.6.1.2.1.2.2.1.7",
								Name: "ifAdminStatus",
							},
						},
						"alias": {
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.2.1.31.1.1.1.18",
								Name: "ifAlias",
							},
						},
						"description": {
							Symbol: SymbolConfig{
								OID:                  "1.3.6.1.2.1.31.1.1.1.1",
								Name:                 "ifName",
								ExtractValue:         "(Row\\d)",
								ExtractValueCompiled: regexp.MustCompile("(Row\\d)"),
							},
						},
						"mac_address": {
							Symbol: SymbolConfig{
								OID:    "1.3.6.1.2.1.2.2.1.6",
								Name:   "ifPhysAddress",
								Format: "mac_address",
							},
						},
						"name": {
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.2.1.31.1.1.1.1",
								Name: "ifName",
							},
						},
						"oper_status": {
							Symbol: SymbolConfig{
								OID:  "1.3.6.1.2.1.2.2.1.8",
								Name: "ifOperStatus",
							},
						},
					},
					IDTags: MetricTagConfigList{
						{
							Tag: "custom-tag",
							Column: SymbolConfig{
								OID:  "1.3.6.1.2.1.31.1.1.1.1",
								Name: "ifAlias",
							},
						},
						{
							Tag: "interface",
							Column: SymbolConfig{
								OID:  "1.3.6.1.2.1.31.1.1.1.1",
								Name: "ifName",
							},
						},
					},
				},
			},
		},
		"another_profile": profileDefinition{
			Metrics: []MetricsConfig{
				{Symbol: SymbolConfig{OID: "1.3.6.1.2.1.1.999.0", Name: "someMetric"}, ForcedType: ""},
			},
			MetricTags: []MetricTagConfig{
				{Tag: "snmp_host2", Column: SymbolConfig{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
				{Tag: "unknown_symbol", OID: "1.3.6.1.2.1.1.999.0", Name: "unknownSymbol"},
			},
			Metadata: MetadataConfig{},
		},
	}
}

func Test_getDefaultProfilesDefinitionFiles(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	actualProfileConfig, err := getDefaultProfilesDefinitionFiles()
	assert.Nil(t, err)

	confdPath := config.Datadog.GetString("confd_path")
	expectedProfileConfig := profileConfigMap{
		"f5-big-ip": {
			DefinitionFile: filepath.Join(confdPath, "snmp.d", "profiles", "f5-big-ip.yaml"),
		},
		"another_profile": {
			DefinitionFile: filepath.Join(confdPath, "snmp.d", "profiles", "another_profile.yaml"),
		},
	}

	assert.Equal(t, expectedProfileConfig, actualProfileConfig)
}

func Test_loadProfiles(t *testing.T) {
	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "conf.d"))
	config.Datadog.Set("confd_path", defaultTestConfdPath)
	defaultProfilesDef, err := getDefaultProfilesDefinitionFiles()
	assert.Nil(t, err)

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "invalid_ext.d"))
	invalidCyclicConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "invalid_cyclic.d"))

	profileWithInvalidExtends, _ := filepath.Abs(filepath.Join("..", "test", "test_profiles", "profile_with_invalid_extends.yaml"))
	invalidYamlProfile, _ := filepath.Abs(filepath.Join("..", "test", "test_profiles", "invalid_yaml_file.yaml"))
	validationErrorProfile, _ := filepath.Abs(filepath.Join("..", "test", "test_profiles", "validation_error.yaml"))
	type logCount struct {
		log   string
		count int
	}
	tests := []struct {
		name                  string
		confdPath             string
		inputProfileConfigMap profileConfigMap
		expectedProfileDefMap profileDefinitionMap
		expectedIncludeErrors []string
		expectedLogs          []logCount
	}{
		{
			name:                  "ok case",
			confdPath:             defaultTestConfdPath,
			inputProfileConfigMap: defaultProfilesDef,
			expectedProfileDefMap: fixtureProfileDefinitionMap(),
			expectedIncludeErrors: []string{},
		},
		{
			name: "failed to read profile",
			inputProfileConfigMap: profileConfigMap{
				"f5-big-ip": {
					DefinitionFile: filepath.Join(string(filepath.Separator), "does", "not", "exist"),
				},
			},
			expectedProfileDefMap: profileDefinitionMap{},
			expectedLogs: []logCount{
				{"[WARN] loadProfiles: failed to read profile definition `f5-big-ip`: failed to read file", 1},
			},
		},
		{
			name: "invalid extends",
			inputProfileConfigMap: profileConfigMap{
				"f5-big-ip": {
					DefinitionFile: profileWithInvalidExtends,
				},
			},
			expectedProfileDefMap: profileDefinitionMap{},
			expectedLogs: []logCount{
				{"[WARN] loadProfiles: failed to expand profile `f5-big-ip`: failed to read file", 1},
			},
		},
		{
			name:      "invalid recursive extends",
			confdPath: profilesWithInvalidExtendConfdPath,
			inputProfileConfigMap: profileConfigMap{
				"f5-big-ip": {
					DefinitionFile: "f5-big-ip.yaml",
				},
			},
			expectedProfileDefMap: profileDefinitionMap{},
			expectedLogs: []logCount{
				{"[WARN] loadProfiles: failed to expand profile `f5-big-ip`", 1},
				{"invalid.yaml", 2},
			},
		},
		{
			name:      "invalid cyclic extends",
			confdPath: invalidCyclicConfdPath,
			inputProfileConfigMap: profileConfigMap{
				"f5-big-ip": {
					DefinitionFile: "f5-big-ip.yaml",
				},
			},
			expectedProfileDefMap: profileDefinitionMap{},
			expectedLogs: []logCount{
				{"[WARN] loadProfiles: failed to expand profile `f5-big-ip`: cyclic profile extend detected, `_extend1.yaml` has already been extended, extendsHistory=`[_extend1.yaml _extend2.yaml]", 1},
			},
		},
		{
			name: "invalid yaml profile",
			inputProfileConfigMap: profileConfigMap{
				"f5-big-ip": {
					DefinitionFile: invalidYamlProfile,
				},
			},
			expectedProfileDefMap: profileDefinitionMap{},
			expectedLogs: []logCount{
				{"failed to read profile definition `f5-big-ip`: failed to unmarshall", 1},
			},
		},
		{
			name: "validation error profile",
			inputProfileConfigMap: profileConfigMap{
				"f5-big-ip": {
					DefinitionFile: validationErrorProfile,
				},
			},
			expectedProfileDefMap: profileDefinitionMap{},
			expectedLogs: []logCount{
				{"cannot compile `match` (`global_metric_tags[\\w)(\\w+)`)", 1},
				{"cannot compile `match` (`table_match[\\w)`)", 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			log.SetupLogger(l, "debug")

			config.Datadog.Set("confd_path", tt.confdPath)

			profiles, err := loadProfiles(tt.inputProfileConfigMap)
			for _, errorMsg := range tt.expectedIncludeErrors {
				assert.Contains(t, err.Error(), errorMsg)
			}

			w.Flush()
			logs := b.String()

			for _, aLogCount := range tt.expectedLogs {
				assert.Equal(t, aLogCount.count, strings.Count(logs, aLogCount.log), logs)
			}

			for _, profile := range profiles {
				normalizeMetrics(profile.Metrics)
			}

			assert.Equal(t, tt.expectedProfileDefMap, profiles)
		})
	}
}

func Test_getMostSpecificOid(t *testing.T) {
	tests := []struct {
		name           string
		oids           []string
		expectedOid    string
		expectedErrror error
	}{
		{
			"one",
			[]string{"1.2.3.4"},
			"1.2.3.4",
			nil,
		},
		{
			"error on empty oids",
			[]string{},
			"",
			fmt.Errorf("cannot get most specific oid from empty list of oids"),
		},
		{
			"error on parsing",
			[]string{"a.1.2.3"},
			"",
			fmt.Errorf("error parsing part `a` for pattern `a.1.2.3`: strconv.Atoi: parsing \"a\": invalid syntax"),
		},
		{
			"most lengthy",
			[]string{"1.3.4", "1.3.4.1.2"},
			"1.3.4.1.2",
			nil,
		},
		{
			"wild card 1",
			[]string{"1.3.4.*", "1.3.4.1"},
			"1.3.4.1",
			nil,
		},
		{
			"wild card 2",
			[]string{"1.3.4.1", "1.3.4.*"},
			"1.3.4.1",
			nil,
		},
		{
			"sample oids",
			[]string{"1.3.6.1.4.1.3375.2.1.3.4.43", "1.3.6.1.4.1.8072.3.2.10"},
			"1.3.6.1.4.1.3375.2.1.3.4.43",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oid, err := getMostSpecificOid(tt.oids)
			assert.Equal(t, tt.expectedErrror, err)
			assert.Equal(t, tt.expectedOid, oid)
		})
	}
}

func Test_resolveProfileDefinitionPath(t *testing.T) {
	SetConfdPathAndCleanProfiles()

	absPath, _ := filepath.Abs(filepath.Join("tmp", "myfile.yaml"))
	tests := []struct {
		name               string
		definitionFilePath string
		expectedPath       string
	}{
		{
			name:               "abs path",
			definitionFilePath: absPath,
			expectedPath:       absPath,
		},
		{
			name:               "relative path",
			definitionFilePath: "myfile.yaml",
			expectedPath:       filepath.Join(config.Datadog.Get("confd_path").(string), "snmp.d", "profiles", "myfile.yaml"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := resolveProfileDefinitionPath(tt.definitionFilePath)
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}

func Test_loadDefaultProfiles(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	globalProfileConfigMap = nil
	defaultProfiles, err := loadDefaultProfiles()
	assert.Nil(t, err)
	defaultProfiles2, err := loadDefaultProfiles()
	assert.Nil(t, err)

	assert.Equal(t, fmt.Sprintf("%p", defaultProfiles), fmt.Sprintf("%p", defaultProfiles2))
}

func Test_loadDefaultProfiles_invalidDir(t *testing.T) {
	invalidPath, _ := filepath.Abs(filepath.Join(".", "tmp", "invalidPath"))
	config.Datadog.Set("confd_path", invalidPath)
	globalProfileConfigMap = nil

	defaultProfiles, err := loadDefaultProfiles()
	assert.Contains(t, err.Error(), "failed to get default profile definitions: failed to read dir")
	assert.Nil(t, defaultProfiles)
}

func Test_loadDefaultProfiles_invalidExtendProfile(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "debug")

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "invalid_ext.d"))
	config.Datadog.Set("confd_path", profilesWithInvalidExtendConfdPath)
	globalProfileConfigMap = nil

	defaultProfiles, err := loadDefaultProfiles()

	w.Flush()
	logs := b.String()
	assert.Nil(t, err)

	assert.Equal(t, 1, strings.Count(logs, "[WARN] loadProfiles: failed to expand profile `f5-big-ip"), logs)
	assert.Equal(t, profileDefinitionMap{}, defaultProfiles)
}

func Test_loadDefaultProfiles_validAndInvalidProfiles(t *testing.T) {
	// Valid profiles should be returned even if some profiles are invalid
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "debug")

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "valid_invalid.d"))
	config.Datadog.Set("confd_path", profilesWithInvalidExtendConfdPath)
	globalProfileConfigMap = nil

	defaultProfiles, err := loadDefaultProfiles()

	for _, profile := range defaultProfiles {
		normalizeMetrics(profile.Metrics)
	}

	w.Flush()
	logs := b.String()
	assert.Nil(t, err)

	assert.Equal(t, 1, strings.Count(logs, "[WARN] loadProfiles: failed to read profile definition `f5-invalid`"), logs)
	assert.Contains(t, defaultProfiles, "f5-big-ip")
	assert.NotContains(t, defaultProfiles, "f5-invalid")
}

func Test_mergeProfileDefinition(t *testing.T) {
	okBaseDefinition := profileDefinition{
		Metrics: []MetricsConfig{
			{Symbol: SymbolConfig{OID: "1.1", Name: "metric1"}, ForcedType: "gauge"},
		},
		MetricTags: []MetricTagConfig{
			{
				Tag:  "tag1",
				OID:  "2.1",
				Name: "tagName1",
			},
		},
		Metadata: MetadataConfig{
			"device": {
				Fields: map[string]MetadataField{
					"vendor": {
						Value: "f5",
					},
					"description": {
						Symbol: SymbolConfig{
							OID:  "1.3.6.1.2.1.1.1.0",
							Name: "sysDescr",
						},
					},
				},
			},
			"interface": {
				Fields: map[string]MetadataField{
					"admin_status": {
						Symbol: SymbolConfig{

							OID:  "1.3.6.1.2.1.2.2.1.7",
							Name: "ifAdminStatus",
						},
					},
				},
				IDTags: MetricTagConfigList{
					{
						Tag: "alias",
						Column: SymbolConfig{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "ifAlias",
						},
					},
				},
			},
		},
	}
	emptyBaseDefinition := profileDefinition{}
	okTargetDefinition := profileDefinition{
		Metrics: []MetricsConfig{
			{Symbol: SymbolConfig{OID: "1.2", Name: "metric2"}, ForcedType: "gauge"},
		},
		MetricTags: []MetricTagConfig{
			{
				Tag:  "tag2",
				OID:  "2.2",
				Name: "tagName2",
			},
		},
		Metadata: MetadataConfig{
			"device": {
				Fields: map[string]MetadataField{
					"name": {
						Symbol: SymbolConfig{
							OID:  "1.3.6.1.2.1.1.5.0",
							Name: "sysName",
						},
					},
				},
			},
			"interface": {
				Fields: map[string]MetadataField{
					"oper_status": {
						Symbol: SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2.1.8",
							Name: "ifOperStatus",
						},
					},
				},
				IDTags: MetricTagConfigList{
					{
						Tag: "interface",
						Column: SymbolConfig{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "ifName",
						},
					},
				},
			},
		},
	}
	tests := []struct {
		name               string
		targetDefinition   profileDefinition
		baseDefinition     profileDefinition
		expectedDefinition profileDefinition
	}{
		{
			name:             "merge case",
			baseDefinition:   copyProfileDefinition(okBaseDefinition),
			targetDefinition: copyProfileDefinition(okTargetDefinition),
			expectedDefinition: profileDefinition{
				Metrics: []MetricsConfig{
					{Symbol: SymbolConfig{OID: "1.2", Name: "metric2"}, ForcedType: "gauge"},
					{Symbol: SymbolConfig{OID: "1.1", Name: "metric1"}, ForcedType: "gauge"},
				},
				MetricTags: []MetricTagConfig{
					{
						Tag:  "tag2",
						OID:  "2.2",
						Name: "tagName2",
					},
					{
						Tag:  "tag1",
						OID:  "2.1",
						Name: "tagName1",
					},
				},
				Metadata: MetadataConfig{
					"device": {
						Fields: map[string]MetadataField{
							"vendor": {
								Value: "f5",
							},
							"name": {
								Symbol: SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
							"description": {
								Symbol: SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
						},
					},
					"interface": {
						Fields: map[string]MetadataField{
							"oper_status": {
								Symbol: SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2.1.8",
									Name: "ifOperStatus",
								},
							},
							"admin_status": {
								Symbol: SymbolConfig{

									OID:  "1.3.6.1.2.1.2.2.1.7",
									Name: "ifAdminStatus",
								},
							},
						},
						IDTags: MetricTagConfigList{
							{
								Tag: "interface",
								Column: SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
							{
								Tag: "alias",
								Column: SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifAlias",
								},
							},
						},
					},
				},
			},
		},
		{
			name:             "empty base definition",
			baseDefinition:   copyProfileDefinition(emptyBaseDefinition),
			targetDefinition: copyProfileDefinition(okTargetDefinition),
			expectedDefinition: profileDefinition{
				Metrics: []MetricsConfig{
					{Symbol: SymbolConfig{OID: "1.2", Name: "metric2"}, ForcedType: "gauge"},
				},
				MetricTags: []MetricTagConfig{
					{
						Tag:  "tag2",
						OID:  "2.2",
						Name: "tagName2",
					},
				},
				Metadata: MetadataConfig{
					"device": {
						Fields: map[string]MetadataField{
							"name": {
								Symbol: SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
						},
					},
					"interface": {
						Fields: map[string]MetadataField{
							"oper_status": {
								Symbol: SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2.1.8",
									Name: "ifOperStatus",
								},
							},
						},
						IDTags: MetricTagConfigList{
							{
								Tag: "interface",
								Column: SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
						},
					},
				},
			},
		},
		{
			name:             "empty taget definition",
			baseDefinition:   copyProfileDefinition(okBaseDefinition),
			targetDefinition: copyProfileDefinition(emptyBaseDefinition),
			expectedDefinition: profileDefinition{
				Metrics: []MetricsConfig{
					{Symbol: SymbolConfig{OID: "1.1", Name: "metric1"}, ForcedType: "gauge"},
				},
				MetricTags: []MetricTagConfig{
					{
						Tag:  "tag1",
						OID:  "2.1",
						Name: "tagName1",
					},
				},
				Metadata: MetadataConfig{
					"device": {
						Fields: map[string]MetadataField{
							"vendor": {
								Value: "f5",
							},
							"description": {
								Symbol: SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
						},
					},
					"interface": {
						Fields: map[string]MetadataField{
							"admin_status": {
								Symbol: SymbolConfig{

									OID:  "1.3.6.1.2.1.2.2.1.7",
									Name: "ifAdminStatus",
								},
							},
						},
						IDTags: MetricTagConfigList{
							{
								Tag: "alias",
								Column: SymbolConfig{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifAlias",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeProfileDefinition(&tt.targetDefinition, &tt.baseDefinition)
			assert.Equal(t, tt.expectedDefinition.Metrics, tt.targetDefinition.Metrics)
			assert.Equal(t, tt.expectedDefinition.MetricTags, tt.targetDefinition.MetricTags)
			assert.Equal(t, tt.expectedDefinition.Metadata, tt.targetDefinition.Metadata)
		})
	}
}
