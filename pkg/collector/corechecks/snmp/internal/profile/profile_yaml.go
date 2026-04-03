// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.yaml.in/yaml/v2"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

const defaultProfilesFolder = "default_profiles"
const userProfilesFolder = "profiles"

var defaultProfilesMu = &sync.Mutex{}

// loadYamlProfiles will load the profiles from disk only once and store it
// in globalProfileConfigMap. The subsequent call to it will return profiles stored in
// globalProfileConfigMap. The mutex will help loading once when `loadYamlProfiles`
// is called by multiple check instances.
func loadYamlProfiles() (ProfileConfigMap, bool, error) {
	defaultProfilesMu.Lock()
	defer defaultProfilesMu.Unlock()

	profileConfigMap := GetGlobalProfileConfigMap()
	if profileConfigMap != nil {
		log.Debugf("load yaml profiles from cache")
		return profileConfigMap, false, nil
	}
	log.Debugf("build yaml profiles")

	userProfiles, haveLegacyUserProfile := getYamlUserProfiles()
	defaultProfiles := getYamlDefaultProfiles()
	profiles := resolveProfiles(userProfiles, defaultProfiles)

	SetGlobalProfileConfigMap(profiles)

	return profiles, haveLegacyUserProfile, nil
}

func getProfileDefinitions(profilesFolder string, isUserProfile bool) (ProfileConfigMap, bool, error) {
	profilesRoot := getProfileConfdRoot(profilesFolder)
	if isUserProfile {
		log.Debugf("Reading user profiles from %s", profilesRoot)
	} else {
		log.Debugf("Reading ootb profiles from %s", profilesRoot)
	}
	files, err := os.ReadDir(profilesRoot)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read profile dir %q: %w", profilesRoot, err)
	}

	profiles := make(ProfileConfigMap)
	var haveLegacyProfile bool
	for _, f := range files {

		fName := f.Name()
		// Skip non yaml profiles
		if !strings.HasSuffix(fName, ".yaml") {
			continue
		}
		profileName := fName[:len(fName)-len(".yaml")]

		absPath := filepath.Join(profilesRoot, fName)
		definition, isLegacyProfile, err := readProfileDefinition(absPath)
		haveLegacyProfile = haveLegacyProfile || isLegacyProfile
		if err != nil {
			log.Warnf("cannot load profile %q: %v", profileName, err)
			continue
		}
		if definition.Name == "" {
			definition.Name = profileName
		}
		profiles[profileName] = ProfileConfig{
			Definition:    *definition,
			IsUserProfile: isUserProfile,
		}
	}
	return profiles, haveLegacyProfile, nil
}

func readProfileDefinition(definitionFile string) (*profiledefinition.ProfileDefinition, bool, error) {
	filePath := resolveProfileDefinitionPath(definitionFile)
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false, fmt.Errorf("unable to read file %q: %w", filePath, err)
	}

	profileDefinition := profiledefinition.NewProfileDefinition()
	err = yaml.Unmarshal(buf, profileDefinition)
	if err != nil {
		isLegacyProfile := errors.Is(err, profiledefinition.ErrLegacySymbolType)
		if isLegacyProfile {
			log.Warnf("found legacy symbol type in profile %q", definitionFile)
		}
		return nil, isLegacyProfile, fmt.Errorf("parse error in file %q: %w", filePath, err)
	}

	isLegacyProfile := profiledefinition.IsLegacyMetrics(profileDefinition.Metrics)
	if isLegacyProfile {
		log.Warnf("found legacy metrics in profile %q", definitionFile)
	}
	return profileDefinition, isLegacyProfile, nil
}

func resolveProfileDefinitionPath(definitionFile string) string {
	if filepath.IsAbs(definitionFile) {
		return definitionFile
	}
	userProfile := filepath.Join(getProfileConfdRoot(userProfilesFolder), definitionFile)
	if filesystem.FileExists(userProfile) {
		return userProfile
	}
	return filepath.Join(getProfileConfdRoot(defaultProfilesFolder), definitionFile)
}

func getProfileConfdRoot(profileFolderName string) string {
	confdPath := pkgconfigsetup.Datadog().GetString("confd_path")
	return filepath.Join(confdPath, "snmp.d", profileFolderName)
}

func getYamlUserProfiles() (ProfileConfigMap, bool) {
	userProfiles, haveLegacyProfile, err := getProfileDefinitions(userProfilesFolder, true)
	if err != nil {
		log.Warnf("failed to load user profile definitions: %s", err)
		return ProfileConfigMap{}, haveLegacyProfile
	}
	return userProfiles, haveLegacyProfile
}

func getYamlDefaultProfiles() ProfileConfigMap {
	userProfiles, _, err := getProfileDefinitions(defaultProfilesFolder, false)
	if err != nil {
		log.Warnf("failed to load default profile definitions: %s", err)
		return ProfileConfigMap{}
	}
	return userProfiles
}
