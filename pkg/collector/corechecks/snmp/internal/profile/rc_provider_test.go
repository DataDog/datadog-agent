// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profile

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteConfigClearsCorrectedProfileError(t *testing.T) {
	const profileName = "rc-updated-profile"
	const otherProfileName = "rc-invalid-profile"
	t.Cleanup(func() {
		profileExpVar.Delete(profileName)
		profileExpVar.Delete(otherProfileName)
	})

	const validProfile = `{
		"profile_definition": {
			"name": "rc-updated-profile",
			"metrics": [{
				"symbols": [{"OID": "1.3.6.1.2.1.2.2.1.10", "name": "ifInOctets"}],
				"metric_tags": [{
					"tag": "interface",
					"symbol": {"OID": "1.3.6.1.2.1.2.2.1.2", "name": "ifDescr"}
				}]
			}]
		}
	}`
	invalidProfile := strings.Replace(validProfile, `"name": "ifDescr"`, `"name": ""`, 1)
	updates := map[string]state.RawConfig{
		"updated-profile-id": {Config: []byte(invalidProfile)},
		"other-profile-id":   {Config: []byte(strings.Replace(invalidProfile, profileName, otherProfileName, 1))},
	}
	provider := &UpdatableProvider{}
	onUpdate := makeOnUpdate(provider)
	applyStateCallback := func(string, state.ApplyStatus) {}

	onUpdate(updates, applyStateCallback)
	require.False(t, provider.HasProfile(profileName))
	require.NotNil(t, profileExpVar.Get(profileName))
	assert.Contains(t, profileExpVar.Get(profileName).String(), "symbol name missing")
	require.NotNil(t, profileExpVar.Get(otherProfileName))
	otherError := profileExpVar.Get(otherProfileName).String()

	updates["updated-profile-id"] = state.RawConfig{Config: []byte(validProfile)}
	onUpdate(updates, applyStateCallback)
	require.True(t, provider.HasProfile(profileName))
	assert.Nil(t, profileExpVar.Get(profileName))
	require.NotNil(t, profileExpVar.Get(otherProfileName))
	assert.Equal(t, otherError, profileExpVar.Get(otherProfileName).String())
	assert.False(t, provider.HasProfile(otherProfileName))

	updates["updated-profile-id"] = state.RawConfig{Config: []byte(invalidProfile)}
	onUpdate(updates, applyStateCallback)
	assert.False(t, provider.HasProfile(profileName))
	require.NotNil(t, profileExpVar.Get(profileName))
	assert.Contains(t, profileExpVar.Get(profileName).String(), "symbol name missing")
}

func TestUnpackRawConfigs(t *testing.T) {
	brokenConfig := state.RawConfig{Config: []byte(`{
		"profile_definition": {
			"name": "broken-profile",
			"metrics": [not valid json]
		}
	}`)}

	someProfile := ProfileConfig{
		Definition: profiledefinition.ProfileDefinition{
			Name: "some-profile",
			Metrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{
					OID:  "1.2.3.0",
					Name: "someMetric",
				}},
			},
		},
		IsUserProfile: true,
	}

	someProfileRaw := state.RawConfig{Config: []byte(`{
		"profile_definition": {
			"name": "some-profile",
			"metrics": [{
				"symbol": {
					"OID": "1.2.3.0",
					"name": "someMetric"
				}
			}]
		}
	}`)}

	profileWithStringScaleFactor := ProfileConfig{
		Definition: profiledefinition.ProfileDefinition{
			Name: "some-profile",
			Metrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{
					OID:         "1.2.3.0",
					Name:        "someMetric",
					ScaleFactor: 1.23,
				}},
			},
		},
		IsUserProfile: true,
	}
	profileRawWithStringScaleFactor := state.RawConfig{Config: []byte(`{
		"profile_definition": {
			"name": "some-profile",
			"metrics": [{
				"symbol": {
					"OID": "1.2.3.0",
					"name": "someMetric",
					"scale_factor_string": "1.23"
				}
			}]
		}
	}`)}

	profileRawWithWrongStringScaleFactor := state.RawConfig{Config: []byte(`{
		"profile_definition": {
			"name": "some-profile",
			"metrics": [{
				"symbol": {
					"OID": "1.2.3.0",
					"name": "someMetric",
					"scale_factor_string": "not a float"
				}
			}]
		}
	}`)}

	type testCase struct {
		name             string
		configs          map[string]state.RawConfig
		expectedProfiles ProfileConfigMap
		expectedErrors   map[string]string
	}

	for _, tc := range []testCase{{
		name: "normal profile",
		configs: map[string]state.RawConfig{
			"some-id": someProfileRaw,
		},
		expectedProfiles: ProfileConfigMap{
			"some-profile": someProfile,
		},
		expectedErrors: nil,
	}, {
		name: "broken profile",
		configs: map[string]state.RawConfig{
			"some-id": brokenConfig,
		},
		expectedProfiles: ProfileConfigMap{},
		expectedErrors: map[string]string{
			"some-id": "could not unmarshal",
		},
	}, {
		name: "duplicate profile",
		configs: map[string]state.RawConfig{
			"id-1": someProfileRaw,
			"id-2": someProfileRaw,
		},
		expectedProfiles: ProfileConfigMap{
			"some-profile": someProfile,
		},
		expectedErrors: map[string]string{
			"id-2": "multiple profiles for name: \"some-profile\"",
		},
	}, {
		name: "multiple problems",
		configs: map[string]state.RawConfig{
			"id-1":   someProfileRaw,
			"id-2":   someProfileRaw,
			"broken": brokenConfig,
		},
		expectedProfiles: ProfileConfigMap{
			"some-profile": someProfile,
		},
		expectedErrors: map[string]string{
			"id-2":   "multiple profiles for name: \"some-profile\"",
			"broken": "could not unmarshal",
		},
	}, {
		name: "profile with string scale factor",
		configs: map[string]state.RawConfig{
			"some-id": profileRawWithStringScaleFactor,
		},
		expectedProfiles: ProfileConfigMap{
			"some-profile": profileWithStringScaleFactor,
		},
		expectedErrors: nil,
	}, {
		name: "profile with wrong string scale factor",
		configs: map[string]state.RawConfig{
			"some-id": profileRawWithWrongStringScaleFactor,
		},
		expectedProfiles: ProfileConfigMap{
			"some-profile": someProfile,
		},
		expectedErrors: map[string]string{
			"some-id": "could not parse scale factor \"not a float\" as float64: strconv.ParseFloat: parsing \"not a float\": invalid syntax",
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			profiles, errors := unpackRawConfigs(tc.configs)
			assert.Equal(t, tc.expectedProfiles, profiles)
			for k, v := range tc.expectedErrors {
				err := errors[k]
				assert.ErrorContains(t, err, v, "expected error %q for key %q", v, k)
			}
			for k, v := range errors {
				if _, ok := tc.expectedErrors[k]; !ok {
					t.Errorf("unexpected error %q for key %q", v, k)
				}
			}
		})
	}
}
