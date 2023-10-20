// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

	profiles, err := loadProfilesV3(nil)
	if err != nil {
		return nil, err
	}

	SetGlobalProfileConfigMap(profiles)
	return profiles, nil
}

func loadProfilesV3(initConfigProfiles ProfileConfigMap) (ProfileConfigMap, error) {
	defaultProfiles, err := getProfilesDefinitionFilesV2(defaultProfilesFolder, false)
	if err != nil {
		// TODO: Return error?
		log.Warnf("failed to get default profile definitions: %s", err)
		defaultProfiles = ProfileConfigMap{}
	}
	userProfiles, err := getProfilesDefinitionFilesV2(userProfilesFolder, true)
	if err != nil {
		// TODO: Return error?
		log.Warnf("failed to get user profile definitions: %s", err)
		userProfiles = ProfileConfigMap{}
	}

	if len(initConfigProfiles) > 0 {
		// TODO: TESTME
		for key, val := range initConfigProfiles {
			userProfiles[key] = val
		}
	}

	resolvedProfiles, err := resolveProfiles(defaultProfiles, userProfiles)
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

func getDefaultProfilesDefinitionFiles() (ProfileConfigMap, error) {
	// Get default profiles
	profiles, err := getProfilesDefinitionFiles(defaultProfilesFolder)
	if err != nil {
		log.Warnf("failed to read default_profiles: %s", err)
		profiles = make(ProfileConfigMap)
	}
	// Get user profiles
	// User profiles have precedence over default profiles
	userProfiles, err := getProfilesDefinitionFiles(userProfilesFolder)
	if err != nil {
		log.Warnf("failed to read user_profiles: %s", err)
	} else {
		for profileName, profileDef := range userProfiles {
			profileDef.IsUserProfile = true
			profiles[profileName] = profileDef
		}
	}
	return profiles, nil
}

func getProfilesDefinitionFiles(profilesFolder string) (ProfileConfigMap, error) {
	profilesRoot := getProfileConfdRoot(profilesFolder)
	files, err := os.ReadDir(profilesRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir `%s`: %v", profilesRoot, err)
	}

	profiles := make(ProfileConfigMap)
	for _, f := range files {
		fName := f.Name()
		// Skip partial profiles
		if strings.HasPrefix(fName, "_") {
			continue
		}
		// Skip non yaml profiles
		if !strings.HasSuffix(fName, ".yaml") {
			continue
		}
		profileName := fName[:len(fName)-len(".yaml")]
		profiles[profileName] = ProfileConfig{DefinitionFile: filepath.Join(profilesRoot, fName)}
	}
	return profiles, nil
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

func getMostSpecificOid(oids []string) (string, error) {
	var mostSpecificParts []int
	var mostSpecificOid string

	if len(oids) == 0 {
		return "", fmt.Errorf("cannot get most specific oid from empty list of oids")
	}

	for _, oid := range oids {
		parts, err := getOidPatternSpecificity(oid)
		if err != nil {
			return "", err
		}
		if len(parts) > len(mostSpecificParts) {
			mostSpecificParts = parts
			mostSpecificOid = oid
			continue
		}
		if len(parts) == len(mostSpecificParts) {
			for i := range mostSpecificParts {
				if parts[i] > mostSpecificParts[i] {
					mostSpecificParts = parts
					mostSpecificOid = oid
				}
			}
		}
	}
	return mostSpecificOid, nil
}

func getOidPatternSpecificity(pattern string) ([]int, error) {
	wildcardKey := -1
	var parts []int
	for _, part := range strings.Split(strings.TrimLeft(pattern, "."), ".") {
		if part == "*" {
			parts = append(parts, wildcardKey)
		} else {
			intPart, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("error parsing part `%s` for pattern `%s`: %v", part, pattern, err)
			}
			parts = append(parts, intPart)
		}
	}
	return parts, nil
}
