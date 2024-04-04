// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

func Test_resolveProfiles(t *testing.T) {

	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "conf.d"))
	config.Datadog.SetWithoutSource("confd_path", defaultTestConfdPath)
	defaultTestConfdProfiles := ProfileConfigMap{}
	userTestConfdProfiles, err := getProfileDefinitions(userProfilesFolder, true)
	require.NoError(t, err)

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "invalid_ext.d"))
	config.Datadog.SetWithoutSource("confd_path", profilesWithInvalidExtendConfdPath)
	profilesWithInvalidExtendProfiles, err := getProfileDefinitions(userProfilesFolder, true)
	require.NoError(t, err)

	invalidCyclicConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "invalid_cyclic.d"))
	config.Datadog.SetWithoutSource("confd_path", invalidCyclicConfdPath)
	invalidCyclicProfiles, err := getProfileDefinitions(userProfilesFolder, true)
	require.NoError(t, err)

	profileWithInvalidExtendsFile, _ := filepath.Abs(filepath.Join("..", "test", "test_profiles", "profile_with_invalid_extends.yaml"))
	profileWithInvalidExtends, err := readProfileDefinition(profileWithInvalidExtendsFile)
	require.NoError(t, err)

	validationErrorProfileFile, _ := filepath.Abs(filepath.Join("..", "test", "test_profiles", "validation_error.yaml"))
	validationErrorProfile, err := readProfileDefinition(validationErrorProfileFile)
	require.NoError(t, err)

	userProfilesCaseConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "user_profiles.d"))
	config.Datadog.SetWithoutSource("confd_path", userProfilesCaseConfdPath)
	userProfilesCaseUserProfiles, err := getProfileDefinitions(userProfilesFolder, true)
	require.NoError(t, err)
	userProfilesCaseDefaultProfiles, err := getProfileDefinitions(defaultProfilesFolder, true)
	require.NoError(t, err)

	type logCount struct {
		log   string
		count int
	}
	tests := []struct {
		name                    string
		userProfiles            ProfileConfigMap
		defaultProfiles         ProfileConfigMap
		expectedProfileDefMap   ProfileConfigMap
		expectedProfileMetrics  []string
		expectedInterfaceIDTags []string
		expectedIncludeErrors   []string
		expectedLogs            []logCount
	}{
		{
			name:                  "ok case",
			userProfiles:          userTestConfdProfiles,
			defaultProfiles:       defaultTestConfdProfiles,
			expectedProfileDefMap: FixtureProfileDefinitionMap(),
			expectedIncludeErrors: []string{},
		},
		{
			name:            "ok user profiles case",
			userProfiles:    userProfilesCaseUserProfiles,
			defaultProfiles: userProfilesCaseDefaultProfiles,
			expectedProfileMetrics: []string{
				"p1:user_p1_metric",
				"p2:default_p2_metric",
				"p3:user_p3_metric",
				"p4:default_p4_metric",
				"p4:user_p4_metric", // user p4 extends default p4
				"p5:user_intermediate1_metric",
				"p5:user_intermediate2_metric",
				"p5:user_intermediate3_metric",
				"p6:user_intermediate1_metric",
				"p6:user_intermediate2_metric",
				"p6:user_intermediate3_metric",
			},
			expectedInterfaceIDTags: []string{
				"p5:interface",
				"p6:interface",
			},
			expectedIncludeErrors: []string{},
		},
		{
			name: "invalid extends",
			userProfiles: ProfileConfigMap{
				"f5-big-ip": {
					Definition:    *profileWithInvalidExtends,
					IsUserProfile: true,
				},
			},
			expectedProfileDefMap: ProfileConfigMap{},
			expectedLogs: []logCount{
				{"[WARN] loadResolveProfiles: failed to expand profile \"f5-big-ip\": extend does not exist: `does_not_exist`", 1},
			},
		},
		{
			name:                  "invalid recursive extends",
			userProfiles:          profilesWithInvalidExtendProfiles,
			expectedProfileDefMap: ProfileConfigMap{},
			expectedLogs: []logCount{
				{"loadResolveProfiles: failed to expand profile \"generic-if\": extend does not exist: `invalid`", 1},
			},
		},
		{
			name:                  "invalid cyclic extends",
			userProfiles:          invalidCyclicProfiles,
			expectedProfileDefMap: ProfileConfigMap{},
			expectedLogs: []logCount{
				{"[WARN] loadResolveProfiles: failed to expand profile \"f5-big-ip\": cyclic profile extend detected", 1},
			},
		},
		{
			name: "validation error profile",
			userProfiles: ProfileConfigMap{
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

			profiles, err := resolveProfiles(tt.userProfiles, tt.defaultProfiles)
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
			if len(tt.expectedProfileMetrics) > 0 {
				var metricsNames []string
				var ifIDTags []string
				for name, profile := range profiles {
					for _, metric := range profile.Definition.Metrics {
						metricsNames = append(metricsNames, fmt.Sprintf("%s:%s", name, metric.Symbol.Name))
					}
					ifMeta, ok := profile.Definition.Metadata["interface"]
					if ok {
						for _, metricTag := range ifMeta.IDTags {
							ifIDTags = append(ifIDTags, fmt.Sprintf("%s:%s", name, metricTag.Tag))
						}
					}
				}
				assert.ElementsMatch(t, tt.expectedProfileMetrics, metricsNames)
				assert.ElementsMatch(t, tt.expectedInterfaceIDTags, ifIDTags)
			} else {
				assert.Equal(t, tt.expectedProfileDefMap, profiles)
			}
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
