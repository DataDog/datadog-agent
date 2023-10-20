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

	profiles, err := loadProfiles(nil, nil)
	if err != nil {
		return nil, err
	}

	SetGlobalProfileConfigMap(profiles)
	return profiles, nil
}

func loadProfiles(initConfigProfiles ProfileConfigMap, userProfiles ProfileConfigMap) (ProfileConfigMap, error) {
	if initConfigProfiles != nil && userProfiles != nil {
		return nil, fmt.Errorf("passing both initConfigProfiles and userProfiles is not expected")
	}

	var err error
	if userProfiles == nil {
		userProfiles, err = getProfilesDefinitionFilesV2(userProfilesFolder, true)
		if err != nil {
			log.Warnf("failed to get user profile definitions: %s", err)
		}
	}
	if userProfiles == nil {
		userProfiles = ProfileConfigMap{}
	}

	if len(initConfigProfiles) > 0 {
		for key, val := range initConfigProfiles {
			userProfiles[key] = val
		}
	}
	defaultProfiles, err := getProfilesDefinitionFilesV2(defaultProfilesFolder, false)
	if err != nil {
		log.Warnf("failed to get default profile definitions: %s", err)
		defaultProfiles = nil
	}
	resolvedProfiles, err := resolveProfiles(userProfiles, defaultProfiles)
	if err != nil {
		return nil, err
	}

	// TODO: FIND BETTER DESIGN
	if len(initConfigProfiles) > 0 {
		filteredResolvedProfiles := ProfileConfigMap{}
		for key, val := range resolvedProfiles {
			if _, ok := initConfigProfiles[key]; !ok {
				continue
			}
			filteredResolvedProfiles[key] = val
		}
		resolvedProfiles = filteredResolvedProfiles
	}

	return resolvedProfiles, nil
}

func getProfilesDefinitionFilesV2(profilesFolder string, isUserProfile bool) (ProfileConfigMap, error) {
	profilesRoot := getProfileConfdRoot(profilesFolder)
	files, err := os.ReadDir(profilesRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir `%s`: %v", profilesRoot, err)
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
			log.Warnf("failed to read dir `%s`: %v", absPath, err)
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
		return nil, fmt.Errorf("failed to read file `%s`: %s", filePath, err)
	}

	profileDefinition := profiledefinition.NewProfileDefinition()
	err = yaml.Unmarshal(buf, profileDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall %q: %v", filePath, err)
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
