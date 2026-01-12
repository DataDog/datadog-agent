// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profile

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
)

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
