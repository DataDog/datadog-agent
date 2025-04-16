// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profile

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func makeMetrics(oids ...string) []profiledefinition.MetricsConfig {
	result := make([]profiledefinition.MetricsConfig, 0, len(oids))
	for _, oid := range oids {
		result = append(result, profiledefinition.MetricsConfig{
			Symbol: profiledefinition.SymbolConfig{
				OID:  oid,
				Name: "Metric " + oid,
			},
		})
	}
	return result
}

func makeTags(oids ...string) []profiledefinition.MetricTagConfig {
	result := make([]profiledefinition.MetricTagConfig, 0, len(oids))
	for _, oid := range oids {
		result = append(result, profiledefinition.MetricTagConfig{
			Tag: "Tag " + oid,
			Symbol: profiledefinition.SymbolConfigCompat{
				OID:  oid,
				Name: "UnusedButRequired",
			},
		})
	}
	return result
}

var defaultProfiles = ProfileConfigMap{
	"_base": ProfileConfig{
		Definition: profiledefinition.ProfileDefinition{
			Metrics:    makeMetrics("1.1.1.0"),
			MetricTags: makeTags("1.1.2.0"),
		},
	},
	"some_device": ProfileConfig{
		Definition: profiledefinition.ProfileDefinition{
			SysObjectIDs: profiledefinition.StringArray{"9.9.9.*"},
			Extends:      []string{"_base"},
			Device:       profiledefinition.DeviceMeta{Vendor: "ACME Exploding Routers Inc."},
			Metrics:      makeMetrics("1.2.1.0"),
			MetricTags:   makeTags("1.2.2.0"),
		},
	},
}.withNames()

var customProfiles = ProfileConfigMap{
	"_base": ProfileConfig{
		Definition: profiledefinition.ProfileDefinition{
			Metrics: makeMetrics("2.1.1.0"),
			Metadata: profiledefinition.MetadataConfig{
				"device": profiledefinition.MetadataResourceConfig{
					Fields: map[string]profiledefinition.MetadataField{
						"os_name": {
							Value: "someOS",
						},
					},
				},
			},
		},
		IsUserProfile: true,
	},
	"custom_device": ProfileConfig{
		Definition: profiledefinition.ProfileDefinition{
			SysObjectIDs: profiledefinition.StringArray{"9.9.9.9.12"},
			Extends:      []string{"_base"},
			Device:       profiledefinition.DeviceMeta{Vendor: "Crisco Systems"},
			Metrics:      makeMetrics("2.2.1.0"),
			MetricTags:   makeTags("2.2.2.0"),
		},
		IsUserProfile: true,
	},
}.withNames()

func TestUpdatableProvider(t *testing.T) {
	var mockClock = clock.NewMock()
	r := &UpdatableProvider{}
	logs := TrapLogs(t, log.DebugLvl)
	r.Update(customProfiles.Clone(), defaultProfiles.Clone(), mockClock.Now())
	// TODO: Provide a better signal here so that we can detect log failures without relying on logging
	if !logs.AssertAbsent(t, "validation error") {
		t.Fatal("Aborting due to validation failures.")
	}
	t.Run("custom inherits custom", func(t *testing.T) {
		profile, err := r.GetProfileForSysObjectID("9.9.9.9.12")
		require.NoError(t, err)
		expected := &ProfileConfig{
			DefinitionFile: "",
			Definition: profiledefinition.ProfileDefinition{
				Name:         "custom_device",
				SysObjectIDs: []string{"9.9.9.9.12"},
				Extends:      []string{"_base"},
				Metrics:      makeMetrics("2.2.1.0", "2.1.1.0"),
				MetricTags:   makeTags("2.2.2.0"),
				Device:       profiledefinition.DeviceMeta{Vendor: "Crisco Systems"},
				Metadata: profiledefinition.MetadataConfig{
					"device": profiledefinition.MetadataResourceConfig{
						Fields: map[string]profiledefinition.MetadataField{
							"os_name": {
								Value: "someOS",
							},
						},
					},
				},
			},
			IsUserProfile: true,
		}
		assert.Equal(t, expected, profile)
	})
	t.Run("default inherits custom", func(t *testing.T) {
		profile, err := r.GetProfileForSysObjectID("9.9.9.10")
		require.NoError(t, err)
		expected := &ProfileConfig{
			DefinitionFile: "",
			Definition: profiledefinition.ProfileDefinition{
				Name:         "some_device",
				SysObjectIDs: []string{"9.9.9.*"},
				Extends:      []string{"_base"},
				Device:       profiledefinition.DeviceMeta{Vendor: "ACME Exploding Routers Inc."},
				Metrics:      makeMetrics("1.2.1.0", "2.1.1.0"),
				MetricTags:   makeTags("1.2.2.0"),

				Metadata: profiledefinition.MetadataConfig{
					"device": profiledefinition.MetadataResourceConfig{
						Fields: map[string]profiledefinition.MetadataField{
							"os_name": {
								Value: "someOS",
							},
						},
					},
				},
			},
			IsUserProfile: false,
		}
		assert.Equal(t, expected, profile)
	})
	assert.Equal(t, mockClock.Now(), r.LastUpdated())
	// Make a new set of profiles and trigger an update
	newUserProfiles := customProfiles.Clone()
	delete(newUserProfiles, "_base")
	customProfile := newUserProfiles["custom_device"]
	customProfile.Definition.Version = 1
	customProfile.Definition.Metrics = append(customProfile.Definition.Metrics, makeMetrics("4.0")...)
	newUserProfiles["custom_device"] = customProfile
	mockClock.Add(time.Minute)
	// Update with the new profiles
	r.Update(newUserProfiles, defaultProfiles.Clone(), mockClock.Now())
	logs.AssertAbsent(t, "validation error")

	t.Run("custom is updated and override of base is deleted", func(t *testing.T) {
		profile, err := r.GetProfileForSysObjectID("9.9.9.9.12")
		require.NoError(t, err)
		expected := &ProfileConfig{
			DefinitionFile: "",
			Definition: profiledefinition.ProfileDefinition{
				Name:         "custom_device",
				SysObjectIDs: []string{"9.9.9.9.12"},
				Extends:      []string{"_base"},
				Metrics:      makeMetrics("2.2.1.0", "4.0", "1.1.1.0"),
				MetricTags:   makeTags("2.2.2.0", "1.1.2.0"),
				Device:       profiledefinition.DeviceMeta{Vendor: "Crisco Systems"},
				Metadata:     profiledefinition.MetadataConfig{},
				Version:      1,
			},
			IsUserProfile: true,
		}
		assert.Equal(t, expected, profile)
	})
}
