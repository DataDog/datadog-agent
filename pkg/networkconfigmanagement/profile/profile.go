// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package profile defines models, logic, functions to load/parse/manage network device profiles
package profile

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"go.yaml.in/yaml/v2"
)

//go:embed default_profiles/*
var defaultProfilesFS embed.FS

// CommandType represent enums for standard CLI command output we are interested in for NCM
type CommandType string

const (
	// Running represents getting the running config for a device
	Running CommandType = "running"
	// Startup represents getting the startup config for a device
	Startup CommandType = "startup"
	// Version represents the command that will give the device's vendor, OS, version, etc.
	Version CommandType = "version"
)

// Commands is a sub-section within `DeviceProfile` that details the specific commands needed to do an intended action
type Commands struct {
	CommandType     CommandType     `json:"type" yaml:"type"`
	Values          []string        `json:"values" yaml:"values"`
	ProcessingRules ProcessingRules `json:"processing_rules" yaml:"processing_rules"`
	Scrubber        *scrubber.Scrubber
}

// Map represents the mapping profile name to profiles from the loaded directory
type Map map[string]*NCMProfile

// Definition represents a common interface that profile types would implement for shared fields
type Definition[T any] interface {
	GetName() string
	SetName(name string)
}

// BaseProfile struct with common fields
type BaseProfile struct {
	Name string `json:"name" yaml:"name"`
}

// GetName retrieves the name of the profile
func (p *BaseProfile) GetName() string { return p.Name }

// SetName will set the name for the profile
func (p *BaseProfile) SetName(name string) { p.Name = name }

// NCMProfileRaw represents the exact profile to unmarshal from the YAML or JSON file
type NCMProfileRaw struct {
	BaseProfile
	Commands []Commands `json:"commands" yaml:"commands"`
}

// NCMProfile represents the profile with transformed variables such as the commands map for easy access to commands
type NCMProfile struct {
	BaseProfile
	Commands map[CommandType]*Commands
}

// GetCommandValues retrieves the list of CLI commands corresponding to the main command (e.g. running) intended for the device
func (np *NCMProfile) GetCommandValues(command CommandType) ([]string, error) {
	cmds, ok := np.Commands[command]
	if !ok {
		return nil, fmt.Errorf("could not find values for the command from the profile %s: %s", np.Name, command)
	}
	return cmds.Values, nil
}

// GetProfile retrieves the profile from the profile map (by name)
func (pm Map) GetProfile(profileName string) (*NCMProfile, error) {
	profile, ok := pm[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}
	return profile, nil
}

var profilesRootOverride string // for testing only

// getNCMProfileFS returns the filesystem to use for profiles
// Returns the embedded FS by default, or allows filesystem override for testing
func getNCMProfileFS() fs.FS {
	if profilesRootOverride != "" {
		return os.DirFS(profilesRootOverride)
	}
	return defaultProfilesFS
}

// SetProfilesPathForTesting allows tests to override the profiles location
func SetProfilesPathForTesting(path string) {
	// path should point to the directory containing the profiles folder
	// e.g., "test/conf.d/network_config_management.d"
	profilesRootOverride = path
}

// ResetProfilesPath resets to use embedded profiles (for test cleanup)
func ResetProfilesPath() {
	profilesRootOverride = ""
}

// GetProfileMap retrieves the map of profiles loaded from the profile folder path given
func GetProfileMap(profilesFolder string) (Map, error) {
	profileFS := getNCMProfileFS()
	folderToRead := profilesFolder
	if folderToRead == "" {
		folderToRead = defaultProfilesFolder
	}

	files, err := fs.ReadDir(profileFS, folderToRead)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile dir %q: %w", folderToRead, err)
	}

	profiles := make(Map)
	for _, file := range files {
		filename := file.Name()
		// Skip non yaml/json profiles
		if !strings.HasSuffix(filename, ".yaml") && !strings.HasSuffix(filename, ".json") {
			continue
		}
		profileName := strings.TrimSuffix(filename, filepath.Ext(filename))
		filePath := path.Join(folderToRead, filename)

		bytes, err := fs.ReadFile(profileFS, filePath)
		if err != nil {
			log.Warnf("cannot read profile file %q: %v", filePath, err)
			continue
		}

		definition, err := parseNCMProfileFromBytes(bytes, profileName)
		if err != nil {
			log.Warnf("cannot load profile %q: %v", profileName, err)
			continue
		}
		if definition.Name == "" {
			definition.Name = profileName
		}
		profiles[profileName] = definition
	}
	return profiles, nil
}

// parseNCMProfileFromBytes parses from bytes instead of file path
func parseNCMProfileFromBytes(b []byte, profileName string) (*NCMProfile, error) {
	var ncmRawProfile NCMProfileRaw

	if json.Valid(b) {
		if err := json.Unmarshal(b, &ncmRawProfile); err == nil {
			return transformRawProfile(&ncmRawProfile), nil
		}
		log.Warnf("unable to parse JSON profile %q", profileName)
	}

	if err := yaml.UnmarshalStrict(b, &ncmRawProfile); err != nil {
		return nil, fmt.Errorf("unable to parse JSON or YAML for profile %q: %w", profileName, err)
	}

	return transformRawProfile(&ncmRawProfile), nil
}

// transformRawProfile converts NCMProfileRaw to NCMProfile
func transformRawProfile(raw *NCMProfileRaw) *NCMProfile {
	np := &NCMProfile{
		BaseProfile: raw.BaseProfile,
		Commands:    make(map[CommandType]*Commands),
	}
	for i := range raw.Commands {
		cmd := &raw.Commands[i]
		cmd.initializeScrubber()
		np.Commands[cmd.CommandType] = cmd
	}
	return np
}

const defaultProfilesFolder = "default_profiles"

// SetConfdPathAndCleanProfiles is used for testing only
func SetConfdPathAndCleanProfiles() {
	file, _ := filepath.Abs(filepath.Join(".", "test", "conf.d", "network_config_management.d"))
	// this is for unit tests running in the profile directory
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join("..", "test", "conf.d", "network_config_management.d"))
	}
	// this is for unit tests running for the core check (`networkconfigmanagement_test.go`)
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join("..", "..", "..", "networkconfigmanagement", "test", "conf.d", "network_config_management.d"))
	}
	SetProfilesPathForTesting(file)
}

// pathExists returns true if the given path exists (taken from SNMP profile helper utils)
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
