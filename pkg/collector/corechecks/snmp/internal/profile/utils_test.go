// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/mohae/deepcopy"
	"github.com/stretchr/testify/assert"
)

func Test_mergeProfiles(t *testing.T) {
	profilesA := ProfileConfigMap{
		"profile-p1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Name:        "profile-p1",
				Description: "profile-p1 from A",
			},
		},
		"profile-p2": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metadata: profiledefinition.MetadataConfig{
					"device": profiledefinition.MetadataResourceConfig{
						Fields: map[string]profiledefinition.MetadataField{
							"name": {
								Value: "foo",
							},
						},
					},
				},
			},
		},
	}
	profilesACopy := deepcopy.Copy(profilesA).(ProfileConfigMap)
	profilesB := ProfileConfigMap{
		"profile-p1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Name:        "profile-p1",
				Description: "profile-p1 from B",
			},
		},
		"profile-p3": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Metadata: profiledefinition.MetadataConfig{
					"device": profiledefinition.MetadataResourceConfig{
						Fields: map[string]profiledefinition.MetadataField{
							"name": {
								Value: "bar",
							},
						},
					},
				},
			},
		},
	}
	profilesBCopy := deepcopy.Copy(profilesB).(ProfileConfigMap)

	actualMergedProfiles := mergeProfiles(profilesA, profilesB)

	p2 := actualMergedProfiles["profile-p2"]
	p2.Definition.Description = "abc"
	p2.Definition.Metadata["device"] = profiledefinition.MetadataResourceConfig{
		Fields: map[string]profiledefinition.MetadataField{
			"name": {
				Value: "foo2",
			},
		},
	}
	actualMergedProfiles["profile-p2"] = p2

	p3 := actualMergedProfiles["profile-p3"]
	p3.Definition.Description = "abc"
	p3.Definition.Metadata["device"] = profiledefinition.MetadataResourceConfig{
		Fields: map[string]profiledefinition.MetadataField{
			"name": {
				Value: "bar2",
			},
		},
	}
	actualMergedProfiles["profile-p3"] = p3

	expectedMergedProfiles := ProfileConfigMap{
		"profile-p1": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Name:        "profile-p1",
				Description: "profile-p1 from B",
			},
		},
		"profile-p2": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Description: "abc",
				Metadata: profiledefinition.MetadataConfig{
					"device": profiledefinition.MetadataResourceConfig{
						Fields: map[string]profiledefinition.MetadataField{
							"name": {
								Value: "foo2",
							},
						},
					},
				},
			},
		},
		"profile-p3": ProfileConfig{
			Definition: profiledefinition.ProfileDefinition{
				Description: "abc",
				Metadata: profiledefinition.MetadataConfig{
					"device": profiledefinition.MetadataResourceConfig{
						Fields: map[string]profiledefinition.MetadataField{
							"name": {
								Value: "bar2",
							},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expectedMergedProfiles, actualMergedProfiles)
	assert.Equal(t, profilesACopy, profilesA)
	assert.Equal(t, profilesBCopy, profilesB)
}
