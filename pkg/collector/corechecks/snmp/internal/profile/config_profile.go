// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"time"
)

// Provider is an interface that provides profiles by name
type Provider interface {
	// HasProfile returns true if and only if we have a profile by this name.
	HasProfile(profileName string) bool
	// GetProfile returns the profile with this name, or nil if there isn't one.
	GetProfile(profileName string) *ProfileConfig
	// GetProfileForSysObjectID returns the best matching profile for this sysObjectID, or nil if there isn't one.
	GetProfileForSysObjectID(sysObjectID string) (*ProfileConfig, error)
	// LastUpdated returns when this Provider last changed
	LastUpdated() time.Time
}

// staticProvider is a static implementation of Provider
type staticProvider struct {
	configMap   ProfileConfigMap
	lastUpdated time.Time
}

func (s *staticProvider) GetProfile(name string) *ProfileConfig {
	if profile, ok := s.configMap[name]; ok {
		return &profile
	}
	return nil
}

func (s *staticProvider) HasProfile(profileName string) bool {
	_, ok := s.configMap[profileName]
	return ok
}

func (s *staticProvider) GetProfileForSysObjectID(sysObjectID string) (*ProfileConfig, error) {
	return getProfileForSysObjectID(s.configMap, sysObjectID)
}

func (s *staticProvider) LastUpdated() time.Time {
	return s.lastUpdated
}

// StaticProvider makes a provider that serves the static data from this config map.
func StaticProvider(profiles ProfileConfigMap) Provider {
	return &staticProvider{
		configMap:   profiles,
		lastUpdated: time.Now(),
	}
}

// ProfileConfigMap is a set of ProfileConfig instances each identified by name.
type ProfileConfigMap map[string]ProfileConfig

// withNames assigns the key names to Definition.Name for every profile. This is for testing.
func (pcm ProfileConfigMap) withNames() ProfileConfigMap {
	for name, profile := range pcm {
		if profile.Definition.Name == "" {
			def := profile.Definition
			def.Name = name
			pcm[name] = ProfileConfig{
				DefinitionFile: profile.DefinitionFile,
				Definition:     def,
				IsUserProfile:  profile.IsUserProfile,
			}
		}
	}
	return pcm
}

// Clone duplicates a ProfileConfigMap
func (pcm ProfileConfigMap) Clone() ProfileConfigMap {
	return profiledefinition.CloneMap(pcm)
}

// ProfileConfig represents a profile configuration.
type ProfileConfig struct {
	DefinitionFile string                              `yaml:"definition_file"`
	Definition     profiledefinition.ProfileDefinition `yaml:"definition"`

	IsUserProfile bool `yaml:"-"`
}

// Clone duplicates a ProfileConfig
func (p ProfileConfig) Clone() ProfileConfig {
	return ProfileConfig{
		DefinitionFile: p.DefinitionFile,
		Definition:     *p.Definition.Clone(),
		IsUserProfile:  p.IsUserProfile,
	}
}
