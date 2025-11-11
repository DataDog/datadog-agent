// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package profile defines models, logic, functions to load/parse/manage network device profiles
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"gopkg.in/yaml.v2"
)

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

// ParseProfileFromFile is a base function to easily unmarshal YAML for any type T given a file path
func ParseProfileFromFile[T Definition[T]](filePath string) (T, error) {
	var profile T
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return profile, fmt.Errorf("unable to read file %q: %w", filePath, err)
	}

	if json.Valid(buf) {
		// if successfully unmarshalled, return early
		if err = json.Unmarshal(buf, &profile); err == nil {
			return profile, nil
		}
		log.Warnf("unable to parse JSON profile from file %q: %v", filePath, err)
	}
	// try to unmarshal as YAML next
	err = yaml.UnmarshalStrict(buf, &profile)
	// err out in this case, not parseable as JSON and YAML
	if err != nil {
		return profile, fmt.Errorf("unable to parse JSON or YAML; parse error in file %q: %w", filePath, err)
	}
	return profile, nil
}

// ParseNCMProfileFromFile does extra work to unmarshal the YAML, transforming the list of commands into a map for ease of retrieval
func ParseNCMProfileFromFile(filePath string) (*NCMProfile, error) {
	path := resolveNCMProfileDefinitionPath(filePath)
	ncmRawProfile, err := ParseProfileFromFile[*NCMProfileRaw](path)
	if err != nil {
		return nil, err
	}
	var np NCMProfile
	np.Commands = make(map[CommandType]*Commands)
	for i := range ncmRawProfile.Commands {
		cmd := &ncmRawProfile.Commands[i]
		// initialize scrubber if redaction rules are specified
		cmd.initializeScrubber()
		np.Commands[cmd.CommandType] = cmd
	}
	return &np, nil
}

// GetCommandValues retrieves the list of CLI commands corresponding to the main command (e.g. running) intended for the device
func (np *NCMProfile) GetCommandValues(command CommandType) ([]string, error) {
	cmds, ok := np.Commands[command]
	if !ok {
		return nil, fmt.Errorf("could not find values for the command from the profile %s: %s", np.Name, command)
	}
	return cmds.Values, nil
}

// GetProfileMap retrieves the map of profiles loaded from the profile folder path given
func GetProfileMap(profilesFolder string) (Map, error) {
	profilesRoot := getNCMProfileConfdRoot(profilesFolder)
	files, err := os.ReadDir(profilesRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile dir %q: %w", profilesRoot, err)
	}

	profiles := make(Map)
	for _, file := range files {
		filename := file.Name()
		// Skip non yaml/json profiles
		if !strings.HasSuffix(filename, ".yaml") && !strings.HasSuffix(filename, ".json") {
			continue
		}
		profileName := filename[:len(filename)-len(".yaml")]
		absPath := filepath.Join(profilesRoot, filename)
		definition, err := ParseNCMProfileFromFile(absPath)
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

// GetProfile retrieves the profile from the profile map (by name)
func (pm Map) GetProfile(profileName string) (*NCMProfile, error) {
	profile, ok := pm[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}
	return profile, nil
}

const defaultProfilesFolder = "default_profiles"

func getNCMProfileConfdRoot(profileFolderName string) string {
	confdPath := pkgconfigsetup.Datadog().GetString("confd_path")
	return filepath.Join(confdPath, "network_config_management.d", profileFolderName)
}

func resolveNCMProfileDefinitionPath(definitionFile string) string {
	if filepath.IsAbs(definitionFile) {
		return definitionFile
	}
	return filepath.Join(getNCMProfileConfdRoot(defaultProfilesFolder), definitionFile)
}

// SetConfdPathAndCleanProfiles is used for testing only (taken from SNMP profile helper utils)
func SetConfdPathAndCleanProfiles() {
	file, _ := filepath.Abs(filepath.Join(".", "test", "conf.d"))
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join("..", "test", "conf.d"))
	}
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join("..", "..", "..", "networkconfigmanagement", "test", "conf.d"))
	}
	pkgconfigsetup.Datadog().SetWithoutSource("confd_path", file)
}

// pathExists returns true if the given path exists (taken from SNMP profile helper utils)
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
