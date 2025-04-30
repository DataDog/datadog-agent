// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

func Test_loadProfiles(t *testing.T) {
	mockConfig := configmock.New(t)
	tests := []struct {
		name                   string
		mockConfd              string
		profiles               ProfileConfigMap
		expectedProfileMetrics []string
		expectedProfileNames   []string
		expectedErr            string
	}{
		{
			name:      "OK Use init config profiles",
			mockConfd: "conf.d",
			profiles: ProfileConfigMap{
				"my-init-config-profile": ProfileConfig{},
				"f5-big-ip": ProfileConfig{ // should have precedence over user profiles
					Definition: profiledefinition.ProfileDefinition{
						Metrics: []profiledefinition.MetricsConfig{
							{
								Symbol: profiledefinition.SymbolConfig{
									OID:  "1.2.3.4",
									Name: "init_config_metric",
								},
							},
						},
					},
				},
			},
			expectedProfileNames: []string{
				"f5-big-ip",
				"my-init-config-profile",
			},
			expectedProfileMetrics: []string{
				"init_config_metric",
			},
		},
		{
			name:      "OK init config contains invalid profiles with warnings logs",
			mockConfd: "conf.d",
			profiles: ProfileConfigMap{
				"my-init-config-profile": ProfileConfig{
					Definition: profiledefinition.ProfileDefinition{
						MetricTags: profiledefinition.MetricTagConfigList{
							{
								Match: "invalidRegex({[",
							},
						},
					},
				},
			},
			expectedProfileNames: []string(nil), // invalid profiles are skipped
		},
		// yaml profiles
		{
			name:      "OK Use yaml profiles",
			mockConfd: "conf.d",
			expectedProfileNames: []string{
				"another_profile",
				"f5-big-ip",
			},
		},
		{
			name:                 "OK contains yaml profiles with warning logs",
			mockConfd:            "does_non_exist.d",
			expectedProfileNames: []string(nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetGlobalProfileConfigMap(nil)
			path, _ := filepath.Abs(filepath.Join("..", "test", tt.mockConfd))
			mockConfig.SetWithoutSource("confd_path", path)

			actualProfiles, err := loadProfiles(tt.profiles)
			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
			}
			var actualProfilesNames []string
			for profileName := range actualProfiles {
				actualProfilesNames = append(actualProfilesNames, actualProfiles[profileName].Definition.Name)
			}
			sort.Strings(actualProfilesNames)
			sort.Strings(tt.expectedProfileNames)
			assert.Equal(t, tt.expectedProfileNames, actualProfilesNames)

			if len(tt.expectedProfileMetrics) > 0 {
				var metricsNames []string
				for _, profile := range actualProfiles {
					for _, metric := range profile.Definition.Metrics {
						metricsNames = append(metricsNames, metric.Symbol.Name)
					}
				}
				assert.ElementsMatch(t, tt.expectedProfileMetrics, metricsNames)
			}
		})
	}
}

func Test_getProfileForSysObjectID(t *testing.T) {
	mockProfiles := ProfileConfigMap{
		"profile1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3.4.*"},
			},
		},
		"profile2": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3.4.10"},
			},
		},
		"profile3": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3.4.5.*"},
			},
		},
	}.withNames()
	mockProfilesWithPatternError := ProfileConfigMap{
		"profile1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3.***.*"},
			},
		},
	}.withNames()
	mockProfilesWithInvalidPatternError := ProfileConfigMap{
		"profile1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3.[.*"},
			},
		},
	}.withNames()
	mockProfilesWithDefaultDuplicateSysobjectid := ProfileConfigMap{
		"profile1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3"},
			},
		},
		"profile2": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3"},
			},
		},
		"profile3": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.4"},
			},
		},
	}.withNames()
	mockProfilesWithUserProfilePrecedenceWithUserProfileFirstInList := ProfileConfigMap{
		"user-profile": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "userMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3"},
			},
			IsUserProfile: true,
		},
		"default-profile": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "defaultMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3"},
			},
		},
	}.withNames()
	mockProfilesWithUserProfilePrecedenceWithDefaultProfileFirstInList := ProfileConfigMap{
		"default-profile": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "defaultMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3"},
			},
		},
		"user-profile": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "userMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3"},
			},
			IsUserProfile: true,
		},
	}.withNames()
	mockProfilesWithUserProfileMatchAllAndMorePreciseDefaultProfile := ProfileConfigMap{
		"default-profile": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "defaultMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.*"},
			},
		},
		"user-profile": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "userMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.*"},
			},
			IsUserProfile: true,
		},
	}.withNames()
	mockProfilesWithUserDuplicateSysobjectid := ProfileConfigMap{
		"profile1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3"},
			},
			IsUserProfile: true,
		},
		"profile2": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3"},
			},
			IsUserProfile: true,
		},
	}.withNames()
	tests := []struct {
		name                string
		profiles            ProfileConfigMap
		sysObjectID         string
		expectedProfileName string
		expectedError       string
	}{
		{
			name:                "found matching profile",
			profiles:            mockProfiles,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.3.4.1",
			expectedProfileName: "profile1",
			expectedError:       "",
		},
		{
			name:                "found more precise matching profile",
			profiles:            mockProfiles,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.3.4.10",
			expectedProfileName: "profile2",
			expectedError:       "",
		},
		{
			name:                "found even more precise matching profile",
			profiles:            mockProfiles,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.3.4.5.11",
			expectedProfileName: "profile3",
			expectedError:       "",
		},
		{
			name:                "user profile have precedence with user first in list",
			profiles:            mockProfilesWithUserProfilePrecedenceWithUserProfileFirstInList,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.3",
			expectedProfileName: "user-profile",
			expectedError:       "",
		},
		{
			name:                "user profile have precedence with default first in list",
			profiles:            mockProfilesWithUserProfilePrecedenceWithDefaultProfileFirstInList,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.3",
			expectedProfileName: "user-profile",
			expectedError:       "",
		},
		{
			name:                "user profile with less specific sysobjectid does not have precedence over a default profiel with more precise sysobjectid",
			profiles:            mockProfilesWithUserProfileMatchAllAndMorePreciseDefaultProfile,
			sysObjectID:         "1.3.999",
			expectedProfileName: "default-profile",
			expectedError:       "",
		},
		{
			name:                "failed to get most specific profile for sysObjectID",
			profiles:            mockProfilesWithPatternError,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.3.4.5.11",
			expectedProfileName: "",
			expectedError:       "failed to get most specific profile for sysObjectID \"1.3.6.1.4.1.3375.2.1.3.4.5.11\", for matched oids [1.3.6.1.4.1.3375.2.1.3.***.*]: error parsing part `***` for pattern `1.3.6.1.4.1.3375.2.1.3.***.*`: strconv.Atoi: parsing \"***\": invalid syntax",
		},
		{
			name:                "invalid pattern", // profiles with invalid patterns are skipped, leading to: cannot get most specific oid from empty list of oids
			profiles:            mockProfilesWithInvalidPatternError,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.3.4.5.11",
			expectedProfileName: "",
			expectedError:       "no profiles found for sysObjectID \"1.3.6.1.4.1.3375.2.1.3.4.5.11\"",
		},
		{
			name:                "duplicate sysobjectid",
			profiles:            mockProfilesWithDefaultDuplicateSysobjectid,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.3",
			expectedProfileName: "",
			expectedError:       "has the same sysObjectID (1.3.6.1.4.1.3375.2.1.3) as",
		},
		{
			name:                "unrelated duplicate sysobjectid should not raise error",
			profiles:            mockProfilesWithDefaultDuplicateSysobjectid,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.4",
			expectedProfileName: "profile3",
			expectedError:       "",
		},
		{
			name:                "duplicate sysobjectid",
			profiles:            mockProfilesWithUserDuplicateSysobjectid,
			sysObjectID:         "1.3.6.1.4.1.3375.2.1.3",
			expectedProfileName: "",
			expectedError:       "has the same sysObjectID (1.3.6.1.4.1.3375.2.1.3) as",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := getProfileForSysObjectID(tt.profiles, tt.sysObjectID)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}
			if tt.expectedProfileName == "" {
				assert.Nil(t, profile)
			} else {
				assert.Equal(t, tt.expectedProfileName, profile.Definition.Name)
			}
		})
	}
}
