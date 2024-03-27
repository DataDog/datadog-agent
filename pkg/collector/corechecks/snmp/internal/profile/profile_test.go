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

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"

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
			coreconfig.Datadog.SetWithoutSource("confd_path", path)

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
