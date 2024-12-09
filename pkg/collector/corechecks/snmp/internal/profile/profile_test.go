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

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

func Test_getProfiles(t *testing.T) {
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
				"my-init-config-profile": ProfileConfig{
					Definition: profiledefinition.ProfileDefinition{
						Name: "my-init-config-profile",
					},
				},
				"f5-big-ip": ProfileConfig{ // should have precedence over user profiles
					Definition: profiledefinition.ProfileDefinition{
						Name: "f5-big-ip",
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
						Name: "my-init-config-profile",
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

		// json profiles.json.gz profiles
		{
			name:      "OK Use json profiles.json.gz profiles",
			mockConfd: "zipprofiles.d",
			expectedProfileNames: []string{
				"def-p1",
				"my-profile-name",
				"profile-from-ui",
			},
		},
		{
			name:        "ERROR Invalid profiles.json.gz profiles",
			mockConfd:   "zipprofiles_err.d",
			expectedErr: "failed to load profiles from json bundle",
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
			pkgconfigsetup.Datadog().SetWithoutSource("confd_path", path)

			actualProfiles, err := GetProfiles(tt.profiles)
			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
			}
			var actualProfilesNames []string
			for profileName := range actualProfiles {
				actualProfilesNames = append(actualProfilesNames, profileName)
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
	}
	mockProfilesWithPatternError := ProfileConfigMap{
		"profile1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3.***.*"},
			},
		},
	}
	mockProfilesWithInvalidPatternError := ProfileConfigMap{
		"profile1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
				},
				SysObjectIDs: profiledefinition.StringArray{"1.3.6.1.4.1.3375.2.1.3.[.*"},
			},
		},
	}
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
	}
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
	}
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
	}
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
	}
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
	}
	tests := []struct {
		name            string
		profiles        ProfileConfigMap
		sysObjectID     string
		expectedProfile string
		expectedError   string
	}{
		{
			name:            "found matching profile",
			profiles:        mockProfiles,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.1",
			expectedProfile: "profile1",
			expectedError:   "",
		},
		{
			name:            "found more precise matching profile",
			profiles:        mockProfiles,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.10",
			expectedProfile: "profile2",
			expectedError:   "",
		},
		{
			name:            "found even more precise matching profile",
			profiles:        mockProfiles,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.5.11",
			expectedProfile: "profile3",
			expectedError:   "",
		},
		{
			name:            "user profile have precedence with user first in list",
			profiles:        mockProfilesWithUserProfilePrecedenceWithUserProfileFirstInList,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3",
			expectedProfile: "user-profile",
			expectedError:   "",
		},
		{
			name:            "user profile have precedence with default first in list",
			profiles:        mockProfilesWithUserProfilePrecedenceWithDefaultProfileFirstInList,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3",
			expectedProfile: "user-profile",
			expectedError:   "",
		},
		{
			name:            "user profile with less specific sysobjectid does not have precedence over a default profiel with more precise sysobjectid",
			profiles:        mockProfilesWithUserProfileMatchAllAndMorePreciseDefaultProfile,
			sysObjectID:     "1.3.999",
			expectedProfile: "default-profile",
			expectedError:   "",
		},
		{
			name:            "failed to get most specific profile for sysObjectID",
			profiles:        mockProfilesWithPatternError,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.5.11",
			expectedProfile: "",
			expectedError:   "failed to get most specific profile for sysObjectID \"1.3.6.1.4.1.3375.2.1.3.4.5.11\", for matched oids [1.3.6.1.4.1.3375.2.1.3.***.*]: error parsing part `***` for pattern `1.3.6.1.4.1.3375.2.1.3.***.*`: strconv.Atoi: parsing \"***\": invalid syntax",
		},
		{
			name:            "invalid pattern", // profiles with invalid patterns are skipped, leading to: cannot get most specific oid from empty list of oids
			profiles:        mockProfilesWithInvalidPatternError,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3.4.5.11",
			expectedProfile: "",
			expectedError:   "failed to get most specific profile for sysObjectID \"1.3.6.1.4.1.3375.2.1.3.4.5.11\", for matched oids []: cannot get most specific oid from empty list of oids",
		},
		{
			name:            "duplicate sysobjectid",
			profiles:        mockProfilesWithDefaultDuplicateSysobjectid,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3",
			expectedProfile: "",
			expectedError:   "has the same sysObjectID (1.3.6.1.4.1.3375.2.1.3) as",
		},
		{
			name:            "unrelated duplicate sysobjectid should not raise error",
			profiles:        mockProfilesWithDefaultDuplicateSysobjectid,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.4",
			expectedProfile: "profile3",
			expectedError:   "",
		},
		{
			name:            "duplicate sysobjectid",
			profiles:        mockProfilesWithUserDuplicateSysobjectid,
			sysObjectID:     "1.3.6.1.4.1.3375.2.1.3",
			expectedProfile: "",
			expectedError:   "has the same sysObjectID (1.3.6.1.4.1.3375.2.1.3) as",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := GetProfileForSysObjectID(tt.profiles, tt.sysObjectID)
			if tt.expectedError == "" {
				assert.Nil(t, err)
			} else {
				assert.Contains(t, err.Error(), tt.expectedError)
			}
			assert.Equal(t, tt.expectedProfile, profile)
		})
	}
}
