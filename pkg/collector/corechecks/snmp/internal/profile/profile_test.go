package profile

import (
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"sort"
	"testing"
)

func Test_getProfiles(t *testing.T) {
	tests := []struct {
		name                 string
		mockConfd            string
		profiles             ProfileConfigMap
		expectedProfileNames []string
		expectedErr          string
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
			},
			expectedProfileNames: []string{
				"my-init-config-profile",
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
				"my-profile-name",
				"profile-from-ui",
			},
		},
		{
			name:        "ERROR Invalid profiles.json.gz profiles",
			mockConfd:   "zipprofiles_err.d",
			expectedErr: "failed to load bundle json profiles",
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
			coreconfig.Datadog.Set("confd_path", path)

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
		})
	}
}
