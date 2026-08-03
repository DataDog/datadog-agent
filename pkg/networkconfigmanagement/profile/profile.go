// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package profile defines models, logic, functions to load/parse/manage network device profiles
package profile

import (
	"fmt"
)

// ProfileName identifies a device profile, e.g. one of the DefaultProfiles keys.
type ProfileName string

// Map represents the mapping profile name to profiles from the loaded directory
type Map map[ProfileName]*NCMProfile

// NCMProfile represents the profile with transformed variables such as the commands map for easy access to commands
type NCMProfile struct {
	Name     ProfileName
	Commands CommandSet
	// Preprocessing is a set of "redactions" that get applied immediately. If
	// you roll back, it will be to the version AFTER preprocessing. This is to
	// remove things like extra trailing/leading whitespace, or text like
	// "Current configuration:" that we don't have options to suppress.
	Preprocessing []RedactionRule
	Redactions    []RedactionRule
	MetadataRules []MetadataRule
}

type PushConfig struct {
	Copy       *SCPCommand
	SetRunning *PlainCommand
	SetStartup *PlainCommand
}

// CanPush returns whether or not this PushConfig can be used - mainly it is
// used for detecting zero values.
func (pc *PushConfig) CanPush() bool {
	if pc == nil {
		return false
	}
	return pc.Copy != nil && pc.SetRunning != nil
}

type CommandSet struct {
	Verify     *PlainCommand
	GetVersion *PlainCommand `json:"get_version,omitempty"`
	// Config fetching
	GetRunning *PlainCommand `json:"get_running,omitempty"`
	GetStartup *PlainCommand `json:"get_startup,omitempty"`
	// Config pushing
	PushConfig *PushConfig
}

// CanPush returns whether or not this CommandSet includes a valid Push command.
func (cs *CommandSet) CanPush() bool {
	return cs.PushConfig.CanPush()
}

// GetProfile retrieves the profile from the profile map (by name)
func (pm Map) GetProfile(profileName ProfileName) (*NCMProfile, error) {
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
