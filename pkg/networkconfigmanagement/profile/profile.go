// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package profile defines models, logic, functions to load/parse/manage network device profiles
package profile

import (
	"fmt"
)

// Map represents the mapping profile name to profiles from the loaded directory
type Map map[string]*NCMProfile

// NCMProfile represents the profile with transformed variables such as the commands map for easy access to commands
type NCMProfile struct {
	Name          string
	Commands      CommandSet
	Redactions    []RedactionRule
	MetadataRules []MetadataRule
}

type CommandSet struct {
	GetVersion *Command `json:"get_version,omitempty"`
	// Config fetching
	GetRunning *Command `json:"get_running,omitempty"`
	GetStartup *Command `json:"get_startup,omitempty"`
	// Config pushing
	// CopyConfigFile takes the config and copies it to the deviced
	CopyConfigFile *SCPCommand `json:"copy_config_file,omitempty"`
	// ReplaceConfig assumes CopyConfigFile has already happened, and replaces the config
	ReplaceConfig *Command `json:"replace_config,omitempty"`
}

// GetProfile retrieves the profile from the profile map (by name)
func (pm Map) GetProfile(profileName string) (*NCMProfile, error) {
	profile, ok := pm[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}
	return profile, nil
}

// TODO move the profile map to the component so that it can be injected and
// easily mocked that way.
var profilesOverride Map // for testing only

// GetProfileMap retrieves the map of profiles loaded from the profile folder path given
func GetProfileMap() (Map, error) {
	if profilesOverride != nil {
		return profilesOverride, nil
	}
	return DefaultProfiles, nil
}
