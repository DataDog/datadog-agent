// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

const defaultProfilesFolder = "default_profiles"
const userProfilesFolder = "profiles"
const profilesJSONGzipFile = "profiles.json.gz"

var defaultProfilesMu = &sync.Mutex{}

// loadYamlProfiles will load the profiles from disk only once and store it
// in globalProfileConfigMap. The subsequent call to it will return profiles stored in
// globalProfileConfigMap. The mutex will help loading once when `loadYamlProfiles`
// is called by multiple check instances.
func loadYamlProfiles() (ProfileConfigMap, error) {
	defaultProfilesMu.Lock()
	defer defaultProfilesMu.Unlock()

	profileConfigMap := GetGlobalProfileConfigMap()
	if profileConfigMap != nil {
		log.Debugf("load yaml profiles from cache")
		return profileConfigMap, nil
	}
	log.Debugf("build yaml profiles")

	profiles, err := resolveProfiles(getYamlUserProfiles(), getYamlDefaultProfiles())
	if err != nil {
		return nil, err
	}

	SetGlobalProfileConfigMap(profiles)
	return profiles, nil
}

func getProfileDefinitions(profilesFolder string, isUserProfile bool) (ProfileConfigMap, error) {
	profilesRoot := getProfileConfdRoot(profilesFolder)
	files, err := os.ReadDir(profilesRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile dir %q: %w", profilesRoot, err)
	}

	profiles := make(ProfileConfigMap)
	for _, f := range files {

		fName := f.Name()
		// Skip non yaml profiles
		if !strings.HasSuffix(fName, ".yaml") {
			continue
		}
		profileName := fName[:len(fName)-len(".yaml")]

		absPath := filepath.Join(profilesRoot, fName)
		definition, err := readProfileDefinition(absPath)
		if err != nil {
			log.Warnf("cannot load profile %q: %v", profileName, err)
			continue
		}
		profiles[profileName] = ProfileConfig{
			Definition:    *definition,
			IsUserProfile: isUserProfile,
		}
	}
	return profiles, nil
}

func readProfileDefinition(definitionFile string) (*profiledefinition.ProfileDefinition, error) {
	filePath := resolveProfileDefinitionPath(definitionFile)
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file %q: %w", filePath, err)
	}

	profileDefinition := profiledefinition.NewProfileDefinition()
	err = yaml.Unmarshal(buf, profileDefinition)
	if err != nil {
		return nil, fmt.Errorf("parse error in file %q: %w", filePath, err)
	}
	return profileDefinition, nil
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
	confdPath := config.Datadog.GetString("confd_path")
	return filepath.Join(confdPath, "snmp.d", profileFolderName)
}

func getYamlUserProfiles() ProfileConfigMap {
	userProfiles, err := getProfileDefinitions(userProfilesFolder, true)
	if err != nil {
		log.Warnf("failed to load user profile definitions: %s", err)
		return ProfileConfigMap{}
	}
	return userProfiles
}

func getYamlDefaultProfiles() ProfileConfigMap {
	userProfiles, err := getProfileDefinitions(defaultProfilesFolder, false)
	if err != nil {
		log.Warnf("failed to load default profile definitions: %s", err)
		return ProfileConfigMap{}
	}
	return userProfiles
}
