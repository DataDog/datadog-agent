package snmp

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	assert "github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func mockProfilesDefinitions() profileDefinitionMap {
	metrics := []metricsConfig{
		{Symbol: symbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.0", Name: "sysStatMemoryTotal"}, ForcedType: "gauge"},
		{Symbol: symbolConfig{OID: "1.3.6.1.4.1.3375.2.1.1.2.1.44.999", Name: "oldSyntax"}},
		{
			ForcedType: "monotonic_count",
			Symbols: []symbolConfig{
				{OID: "1.3.6.1.2.1.2.2.1.14", Name: "ifInErrors"},
				{OID: "1.3.6.1.2.1.2.2.1.13", Name: "ifInDiscards"},
			},
			MetricTags: []metricTagConfig{
				{Tag: "interface", Column: symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.1", Name: "ifName"}},
				{Tag: "interface_alias", Column: symbolConfig{OID: "1.3.6.1.2.1.31.1.1.1.18", Name: "ifAlias"}},
			},
		},
		{Symbol: symbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
	}
	return profileDefinitionMap{"f5-big-ip": profileDefinition{
		Metrics:      metrics,
		Extends:      []string{"_base.yaml", "_generic-if.yaml"},
		Device:       deviceMeta{Vendor: "f5"},
		SysObjectIds: StringArray{"1.3.6.1.4.1.3375.2.1.3.4.*"},
		MetricTags: []metricTagConfig{
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
			{Tag: "snmp_host", Index: 0x0, Column: symbolConfig{OID: "", Name: ""}, OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
		},
	}}
}

func Test_getDefaultProfilesDefinitionFiles(t *testing.T) {
	setConfdPathAndCleanProfiles()
	actualProfileConfig, err := getDefaultProfilesDefinitionFiles()
	assert.Nil(t, err)

	confdPath := config.Datadog.GetString("confd_path")
	expectedProfileConfig := profileConfigMap{
		"f5-big-ip": {
			filepath.Join(confdPath, "snmp.d", "profiles", "f5-big-ip.yaml"),
		},
	}

	assert.Equal(t, expectedProfileConfig, actualProfileConfig)
}

func Test_loadProfiles(t *testing.T) {
	defaultTestConfdPath, _ := filepath.Abs(filepath.Join(".", "test", "conf.d"))
	config.Datadog.Set("confd_path", defaultTestConfdPath)
	defaultProfilesDef, err := getDefaultProfilesDefinitionFiles()
	assert.Nil(t, err)

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join(".", "test", "invalid_ext_conf.d"))
	invalidCyclicConfdPath, _ := filepath.Abs(filepath.Join(".", "test", "invalid_cyclic_conf.d"))

	profileWithInvalidExtends, _ := filepath.Abs(filepath.Join(".", "test", "test_profiles", "profile_with_invalid_extends.yaml"))
	invalidYamlProfile, _ := filepath.Abs(filepath.Join(".", "test", "test_profiles", "invalid_yaml_file.yaml"))
	validationErrorProfile, _ := filepath.Abs(filepath.Join(".", "test", "test_profiles", "validation_error.yaml"))
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
			name:                  "default",
			confdPath:             defaultTestConfdPath,
			inputProfileConfigMap: defaultProfilesDef,
			expectedProfileDefMap: mockProfilesDefinitions(),
			expectedIncludeErrors: []string{},
		},
		{
			name: "failed to read profile",
			inputProfileConfigMap: profileConfigMap{
				"f5-big-ip": {
					filepath.Join(string(filepath.Separator), "does", "not", "exist"),
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
					profileWithInvalidExtends,
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
					"f5-big-ip.yaml",
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
					"f5-big-ip.yaml",
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
					invalidYamlProfile,
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
					validationErrorProfile,
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
	setConfdPathAndCleanProfiles()

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
	setConfdPathAndCleanProfiles()
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

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join(".", "test", "invalid_ext_conf.d"))
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

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join(".", "test", "valid_invalid_conf.d"))
	config.Datadog.Set("confd_path", profilesWithInvalidExtendConfdPath)
	globalProfileConfigMap = nil

	defaultProfiles, err := loadDefaultProfiles()

	for _, profile := range defaultProfiles {
		normalizeMetrics(profile.Metrics)
	}

	w.Flush()
	logs := b.String()
	assert.Nil(t, err)

	assert.Equal(t, 1, strings.Count(logs, "[WARN] loadProfiles: failed to read profile definition `f5-big-ip-invalid`"), logs)
	assert.Equal(t, mockProfilesDefinitions(), defaultProfiles)
}
