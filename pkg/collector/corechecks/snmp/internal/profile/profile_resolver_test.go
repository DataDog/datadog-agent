// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"bufio"
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"strings"
	"testing"
)

func Test_resolveProfiles(t *testing.T) {

	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "conf.d"))
	config.Datadog.Set("confd_path", defaultTestConfdPath)
	defaultTestConfdProfiles, err := getProfilesDefinitionFilesV2(defaultProfilesFolder, false)
	require.NoError(t, err)
	userTestConfdProfiles, err := getProfilesDefinitionFilesV2(userProfilesFolder, true)
	require.NoError(t, err)

	//defaultProfilesDef, err := getDefaultProfilesDefinitionFiles()
	//assert.Nil(t, err)

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "invalid_ext.d"))
	config.Datadog.Set("confd_path", profilesWithInvalidExtendConfdPath)
	profilesWithInvalidExtendProfiles, err := getProfilesDefinitionFilesV2(userProfilesFolder, true)
	require.NoError(t, err)

	invalidCyclicConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "invalid_cyclic.d"))
	config.Datadog.Set("confd_path", invalidCyclicConfdPath)
	invalidCyclicProfiles, err := getProfilesDefinitionFilesV2(userProfilesFolder, true)
	require.NoError(t, err)

	profileWithInvalidExtendsFile, _ := filepath.Abs(filepath.Join("..", "test", "test_profiles", "profile_with_invalid_extends.yaml"))
	profileWithInvalidExtends, err := readProfileDefinition(profileWithInvalidExtendsFile)
	require.NoError(t, err)

	//invalidYamlFile, _ := filepath.Abs(filepath.Join("..", "test", "test_profiles", "invalid_yaml_file.yaml"))
	//invalidYamlProfile, err := readProfileDefinition(invalidYamlFile)
	//require.NoError(t, err)

	//invalidYamlProfile, _ := filepath.Abs(filepath.Join("..", "test", "test_profiles", "invalid_yaml_file.yaml"))
	validationErrorProfileFile, _ := filepath.Abs(filepath.Join("..", "test", "test_profiles", "validation_error.yaml"))
	validationErrorProfile, err := readProfileDefinition(validationErrorProfileFile)
	require.NoError(t, err)

	type logCount struct {
		log   string
		count int
	}
	tests := []struct {
		name                    string
		profileConfigMap        ProfileConfigMap
		defaultProfileConfigMap ProfileConfigMap
		expectedProfileDefMap   ProfileConfigMap
		expectedIncludeErrors   []string
		expectedLogs            []logCount
	}{
		{
			name:                    "ok case",
			profileConfigMap:        userTestConfdProfiles,
			defaultProfileConfigMap: defaultTestConfdProfiles,
			expectedProfileDefMap:   FixtureProfileDefinitionMap(),
			expectedIncludeErrors:   []string{},
		},
		{
			name: "invalid extends",
			profileConfigMap: ProfileConfigMap{
				"f5-big-ip": {
					Definition:    *profileWithInvalidExtends,
					IsUserProfile: true,
				},
			},
			expectedProfileDefMap: ProfileConfigMap{},
			expectedLogs: []logCount{
				{"[WARN] loadResolveProfiles: failed to expand profile `f5-big-ip`: extend does not exist: `does_not_exist`", 1},
			},
		},
		{
			name:                  "invalid recursive extends",
			profileConfigMap:      profilesWithInvalidExtendProfiles,
			expectedProfileDefMap: ProfileConfigMap{},
			expectedLogs: []logCount{
				{"loadResolveProfiles: failed to expand profile `_generic-if`: extend does not exist: `invalid`", 1},
			},
		},
		{
			name: "invalid cyclic extends",
			//confdPath: invalidCyclicConfdPath,
			profileConfigMap:      invalidCyclicProfiles,
			expectedProfileDefMap: ProfileConfigMap{},
			expectedLogs: []logCount{
				{"[WARN] loadResolveProfiles: failed to expand profile `_extend1`: cyclic profile extend detected", 1},
			},
		},
		//{
		// TODO: Test in profile_yaml_test.go

		//name: "invalid yaml profile",
		//profileConfigMap: ProfileConfigMap{
		//	"f5-big-ip": {
		//		Definition: *invalidYamlProfile,
		//	},
		//},
		//expectedProfileDefMap: ProfileConfigMap{},
		//expectedLogs: []logCount{
		//	{"[WARN] loadResolveProfiles: failed to expand profile `f5-big-ip`: extend does not exist: `does_not_exist`", 1},
		//},
		//},
		{
			name: "validation error profile",
			profileConfigMap: ProfileConfigMap{
				"f5-big-ip": {
					Definition: *validationErrorProfile,
				},
			},
			expectedProfileDefMap: ProfileConfigMap{},
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

			//config.Datadog.Set("confd_path", tt.confdPath)

			profiles, err := resolveProfiles(tt.profileConfigMap, tt.defaultProfileConfigMap)
			for _, errorMsg := range tt.expectedIncludeErrors {
				assert.Contains(t, err.Error(), errorMsg)
			}

			w.Flush()
			logs := b.String()

			for _, aLogCount := range tt.expectedLogs {
				assert.Equal(t, aLogCount.count, strings.Count(logs, aLogCount.log), logs)
			}

			for i, profile := range profiles {
				profiledefinition.NormalizeMetrics(profile.Definition.Metrics)
				profile.DefinitionFile = ""
				profiles[i] = profile
			}

			assert.Equal(t, tt.expectedProfileDefMap, profiles)
		})
	}
}

func Test_mergeProfileDefinition(t *testing.T) {
	okBaseDefinition := profiledefinition.ProfileDefinition{
		Metrics: []profiledefinition.MetricsConfig{
			{Symbol: profiledefinition.SymbolConfig{OID: "1.1", Name: "metric1"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
		},
		MetricTags: []profiledefinition.MetricTagConfig{
			{
				Tag:    "tag1",
				Symbol: profiledefinition.SymbolConfigCompat{OID: "2.1", Name: "tagName1"},
			},
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
				},
				IDTags: profiledefinition.MetricTagConfigList{
					{
						Tag: "alias",
						Symbol: profiledefinition.SymbolConfigCompat{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "ifAlias",
						},
					},
				},
			},
		},
	}
	emptyBaseDefinition := profiledefinition.ProfileDefinition{}
	okTargetDefinition := profiledefinition.ProfileDefinition{
		Metrics: []profiledefinition.MetricsConfig{
			{Symbol: profiledefinition.SymbolConfig{OID: "1.2", Name: "metric2"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
		},
		MetricTags: []profiledefinition.MetricTagConfig{
			{
				Tag:    "tag2",
				Symbol: profiledefinition.SymbolConfigCompat{OID: "2.2", Name: "tagName2"},
			},
		},
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"name": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.1.5.0",
							Name: "sysName",
						},
					},
				},
			},
			"interface": {
				Fields: map[string]profiledefinition.MetadataField{
					"oper_status": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2.1.8",
							Name: "ifOperStatus",
						},
					},
				},
				IDTags: profiledefinition.MetricTagConfigList{
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
	}
	tests := []struct {
		name               string
		targetDefinition   profiledefinition.ProfileDefinition
		baseDefinition     profiledefinition.ProfileDefinition
		expectedDefinition profiledefinition.ProfileDefinition
	}{
		{
			name:             "merge case",
			baseDefinition:   CopyProfileDefinition(okBaseDefinition),
			targetDefinition: CopyProfileDefinition(okTargetDefinition),
			expectedDefinition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2", Name: "metric2"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
					{Symbol: profiledefinition.SymbolConfig{OID: "1.1", Name: "metric1"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
				},
				MetricTags: []profiledefinition.MetricTagConfig{
					{
						Tag:    "tag2",
						Symbol: profiledefinition.SymbolConfigCompat{OID: "2.2", Name: "tagName2"},
					},
					{
						Tag:    "tag1",
						Symbol: profiledefinition.SymbolConfigCompat{OID: "2.1", Name: "tagName1"},
					},
				},
				Metadata: profiledefinition.MetadataConfig{
					"device": {
						Fields: map[string]profiledefinition.MetadataField{
							"vendor": {
								Value: "f5",
							},
							"name": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
							"description": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.1.0",
									Name: "sysDescr",
								},
							},
						},
					},
					"interface": {
						Fields: map[string]profiledefinition.MetadataField{
							"oper_status": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2.1.8",
									Name: "ifOperStatus",
								},
							},
							"admin_status": {
								Symbol: profiledefinition.SymbolConfig{

									OID:  "1.3.6.1.2.1.2.2.1.7",
									Name: "ifAdminStatus",
								},
							},
						},
						IDTags: profiledefinition.MetricTagConfigList{
							{
								Tag: "interface",
								Symbol: profiledefinition.SymbolConfigCompat{
									OID:  "1.3.6.1.2.1.31.1.1.1.1",
									Name: "ifName",
								},
							},
							{
								Tag: "alias",
								Symbol: profiledefinition.SymbolConfigCompat{
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
			baseDefinition:   CopyProfileDefinition(emptyBaseDefinition),
			targetDefinition: CopyProfileDefinition(okTargetDefinition),
			expectedDefinition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2", Name: "metric2"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
				},
				MetricTags: []profiledefinition.MetricTagConfig{
					{
						Tag:    "tag2",
						Symbol: profiledefinition.SymbolConfigCompat{OID: "2.2", Name: "tagName2"},
					},
				},
				Metadata: profiledefinition.MetadataConfig{
					"device": {
						Fields: map[string]profiledefinition.MetadataField{
							"name": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.1.5.0",
									Name: "sysName",
								},
							},
						},
					},
					"interface": {
						Fields: map[string]profiledefinition.MetadataField{
							"oper_status": {
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.3.6.1.2.1.2.2.1.8",
									Name: "ifOperStatus",
								},
							},
						},
						IDTags: profiledefinition.MetricTagConfigList{
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
		},
		{
			name:             "empty taget definition",
			baseDefinition:   CopyProfileDefinition(okBaseDefinition),
			targetDefinition: CopyProfileDefinition(emptyBaseDefinition),
			expectedDefinition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.1", Name: "metric1"}, MetricType: profiledefinition.ProfileMetricTypeGauge},
				},
				MetricTags: []profiledefinition.MetricTagConfig{
					{
						Tag:    "tag1",
						Symbol: profiledefinition.SymbolConfigCompat{OID: "2.1", Name: "tagName1"},
					},
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
						},
						IDTags: profiledefinition.MetricTagConfigList{
							{
								Tag: "alias",
								Symbol: profiledefinition.SymbolConfigCompat{
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
