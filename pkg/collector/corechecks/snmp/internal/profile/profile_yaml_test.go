// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getMetricFromProfile(p profiledefinition.ProfileDefinition, metricName string) *profiledefinition.MetricsConfig {
	for _, m := range p.Metrics {
		if m.Symbol.Name == metricName {
			return &m
		}
	}
	return nil
}

func Test_resolveProfileDefinitionPath(t *testing.T) {
	mockConfig := configmock.New(t)
	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "user_profiles.d"))
	mockConfig.SetInTest("confd_path", defaultTestConfdPath)

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
			name:               "relative path with default profile",
			definitionFilePath: "p2.yaml",
			expectedPath:       filepath.Join(mockConfig.Get("confd_path").(string), "snmp.d", "default_profiles", "p2.yaml"),
		},
		{
			name:               "relative path with user profile",
			definitionFilePath: "p3.yaml",
			expectedPath:       filepath.Join(mockConfig.Get("confd_path").(string), "snmp.d", "profiles", "p3.yaml"),
		},
		{
			name:               "relative path with user profile precedence",
			definitionFilePath: "p1.yaml",
			expectedPath:       filepath.Join(mockConfig.Get("confd_path").(string), "snmp.d", "profiles", "p1.yaml"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := resolveProfileDefinitionPath(tt.definitionFilePath)
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}

func Test_loadYamlProfiles(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	SetGlobalProfileConfigMap(nil)
	defaultProfiles, haveLegacyProfile, err := loadYamlProfiles()
	assert.Nil(t, err)
	defaultProfiles2, haveLegacyProfile2, err := loadYamlProfiles()
	assert.Nil(t, err)

	assert.Equal(t, fmt.Sprintf("%p", defaultProfiles), fmt.Sprintf("%p", defaultProfiles2))
	assert.Equal(t, haveLegacyProfile, haveLegacyProfile2)
}

func Test_loadYamlProfiles_withUserProfiles(t *testing.T) {
	mockConfig := configmock.New(t)
	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "user_profiles.d"))
	SetGlobalProfileConfigMap(nil)
	mockConfig.SetInTest("confd_path", defaultTestConfdPath)

	defaultProfiles, haveLegacyProfile, err := loadYamlProfiles()
	assert.Nil(t, err)

	assert.Len(t, defaultProfiles, 6)
	assert.NotNil(t, defaultProfiles)
	assert.False(t, haveLegacyProfile)

	p1 := defaultProfiles["p1"].Definition // user p1 overrides datadog p1
	p2 := defaultProfiles["p2"].Definition // datadog p2
	p3 := defaultProfiles["p3"].Definition // user p3
	p4 := defaultProfiles["p4"].Definition // user p3

	assert.Equal(t, "p1_user", p1.Device.Vendor) // overrides datadog p1 profile
	assert.NotNil(t, getMetricFromProfile(p1, "user_p1_metric"))

	assert.Equal(t, "p2_datadog", p2.Device.Vendor)
	assert.NotNil(t, getMetricFromProfile(p2, "default_p2_metric"))

	assert.Equal(t, "p3_user", p3.Device.Vendor)
	assert.NotNil(t, getMetricFromProfile(p3, "user_p3_metric"))

	assert.Equal(t, "p4_user", p4.Device.Vendor)
	assert.NotNil(t, getMetricFromProfile(p4, "user_p4_metric"))
	assert.NotNil(t, getMetricFromProfile(p4, "default_p4_metric"))
}

func Test_loadYamlProfiles_invalidDir(t *testing.T) {
	mockConfig := configmock.New(t)
	invalidPath, _ := filepath.Abs(filepath.Join(".", "tmp", "invalidPath"))
	mockConfig.SetInTest("confd_path", invalidPath)
	SetGlobalProfileConfigMap(nil)

	defaultProfiles, haveLegacyProfile, err := loadYamlProfiles()
	assert.Nil(t, err)
	assert.Len(t, defaultProfiles, 0)
	assert.False(t, haveLegacyProfile)
}

func Test_loadYamlProfiles_invalidExtendProfile(t *testing.T) {
	mockConfig := configmock.New(t)
	logs := TrapLogs(t, log.DebugLvl)

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "invalid_ext.d"))
	mockConfig.SetInTest("confd_path", profilesWithInvalidExtendConfdPath)
	SetGlobalProfileConfigMap(nil)

	defaultProfiles, haveLegacyProfile, err := loadYamlProfiles()
	require.NoError(t, err)

	logs.AssertPresent(t, "failed to expand profile \"f5-big-ip\"")
	assert.Equal(t, ProfileConfigMap{}, defaultProfiles)
	assert.False(t, haveLegacyProfile)
}

func Test_loadYamlProfiles_userAndDefaultProfileFolderDoesNotExist(t *testing.T) {
	mockConfig := configmock.New(t)
	logs := TrapLogs(t, log.DebugLvl)

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "does-not-exist.d"))
	mockConfig.SetInTest("confd_path", profilesWithInvalidExtendConfdPath)
	SetGlobalProfileConfigMap(nil)

	defaultProfiles, haveLegacyProfile, err := loadYamlProfiles()
	require.NoError(t, err)

	logs.AssertPresent(t,
		"[WARN] getYamlUserProfiles: failed to load user profile definitions",
		"[WARN] getYamlDefaultProfiles: failed to load default profile definitions",
	)

	assert.Equal(t, ProfileConfigMap{}, defaultProfiles)
	assert.False(t, haveLegacyProfile)
}

func Test_loadYamlProfiles_validAndInvalidProfiles(t *testing.T) {
	mockConfig := configmock.New(t)
	// Valid profiles should be returned even if some profiles are invalid
	logs := TrapLogs(t, log.DebugLvl)

	profilesWithInvalidExtendConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "valid_invalid.d"))
	mockConfig.SetInTest("confd_path", profilesWithInvalidExtendConfdPath)
	SetGlobalProfileConfigMap(nil)

	defaultProfiles, haveLegacyProfile, err := loadYamlProfiles()
	require.NoError(t, err)

	for _, profile := range defaultProfiles {
		profiledefinition.NormalizeMetrics(profile.Definition.Metrics)
	}

	logs.AssertPresent(t, "unmarshal errors")

	assert.Contains(t, defaultProfiles, "f5-big-ip")
	assert.NotContains(t, defaultProfiles, "f5-invalid")
	assert.True(t, haveLegacyProfile)
}

func Test_getProfileDefinitions_legacyProfiles(t *testing.T) {
	mockConfig := configmock.New(t)

	legacyNoOIDLogs := TrapLogs(t, log.DebugLvl)
	legacyNoOIDProfilesConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "legacy_no_oid.d"))
	mockConfig.SetInTest("confd_path", legacyNoOIDProfilesConfdPath)
	SetGlobalProfileConfigMap(nil)
	defaultProfiles, haveLegacyProfile, err := getProfileDefinitions(userProfilesFolder, true)
	require.NoError(t, err)
	assert.Len(t, defaultProfiles, 2)
	assert.Contains(t, defaultProfiles, "legacy")
	assert.Contains(t, defaultProfiles, "valid")
	assert.True(t, haveLegacyProfile)
	legacyNoOIDLogs.AssertPresent(t, "found legacy metrics in profile")

	legacySymbolTypeLogs := TrapLogs(t, log.DebugLvl)
	legacySymbolTypeProfilesConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "legacy_symbol_type.d"))
	mockConfig.SetInTest("confd_path", legacySymbolTypeProfilesConfdPath)
	SetGlobalProfileConfigMap(nil)
	defaultProfiles, haveLegacyProfile, err = getProfileDefinitions(userProfilesFolder, true)
	require.NoError(t, err)
	assert.Len(t, defaultProfiles, 1)
	assert.Contains(t, defaultProfiles, "valid")
	assert.True(t, haveLegacyProfile)
	legacySymbolTypeLogs.AssertPresent(t, "found legacy symbol type in profile")
}

func Test_loadYamlProfiles_legacyProfiles(t *testing.T) {
	mockConfig := configmock.New(t)

	legacyNoOIDLogs := TrapLogs(t, log.DebugLvl)
	legacyNoOIDProfilesConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "legacy_no_oid.d"))
	mockConfig.SetInTest("confd_path", legacyNoOIDProfilesConfdPath)
	SetGlobalProfileConfigMap(nil)
	defaultProfiles, haveLegacyProfile, err := loadYamlProfiles()
	require.NoError(t, err)
	assert.Len(t, defaultProfiles, 1)
	assert.Contains(t, defaultProfiles, "valid")
	assert.True(t, haveLegacyProfile)
	legacyNoOIDLogs.AssertPresent(t, "found legacy metrics in profile")

	legacySymbolTypeLogs := TrapLogs(t, log.DebugLvl)
	legacySymbolTypeProfilesConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "legacy_symbol_type.d"))
	mockConfig.SetInTest("confd_path", legacySymbolTypeProfilesConfdPath)
	SetGlobalProfileConfigMap(nil)
	defaultProfiles, haveLegacyProfile, err = loadYamlProfiles()
	require.NoError(t, err)
	assert.Len(t, defaultProfiles, 1)
	assert.Contains(t, defaultProfiles, "valid")
	assert.True(t, haveLegacyProfile)
	legacySymbolTypeLogs.AssertPresent(t, "found legacy symbol type in profile")
}
