// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
)

const defaultProfilesFolder = "default_profiles"
const userProfilesFolder = "profiles"

var defaultProfilesMu = &sync.Mutex{}

var globalProfileConfigMap profileConfigMap

// loadDefaultProfiles will load the profiles from disk only once and store it
// in globalProfileConfigMap. The subsequent call to it will return profiles stored in
// globalProfileConfigMap. The mutex will help loading once when `loadDefaultProfiles`
// is called by multiple check instances.
func loadDefaultProfiles() (profileConfigMap, error) {
	defaultProfilesMu.Lock()
	defer defaultProfilesMu.Unlock()

	if globalProfileConfigMap != nil {
		log.Debugf("loader default profiles from cache")
		return globalProfileConfigMap, nil
	}
	log.Debugf("build default profiles")

	pConfig, err := getDefaultProfilesDefinitionFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get default profile definitions: %s", err)
	}
	profiles, err := loadProfiles(pConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load default profiles: %s", err)
	}
	globalProfileConfigMap = profiles
	return profiles, nil
}

func getDefaultProfilesDefinitionFiles() (profileConfigMap, error) {
	// Get default profiles
	profiles, err := getProfilesDefinitionFiles(defaultProfilesFolder)
	if err != nil {
		log.Warnf("failed to read default_profiles: %s", err)
		profiles = make(profileConfigMap)
	}
	// Get user profiles
	// User profiles have precedence over default profiles
	userProfiles, err := getProfilesDefinitionFiles(userProfilesFolder)
	if err != nil {
		log.Warnf("failed to read user_profiles: %s", err)
	} else {
		for profileName, profileDef := range userProfiles {
			profileDef.isUserProfile = true
			profiles[profileName] = profileDef
		}
	}
	return profiles, nil
}

func getProfilesDefinitionFiles(profilesFolder string) (profileConfigMap, error) {
	profilesRoot := getProfileConfdRoot(profilesFolder)
	files, err := os.ReadDir(profilesRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir `%s`: %v", profilesRoot, err)
	}

	profiles := make(profileConfigMap)
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
		profiles[profileName] = profileConfig{DefinitionFile: filepath.Join(profilesRoot, fName)}
	}
	return profiles, nil
}

func loadProfiles(pConfig profileConfigMap) (profileConfigMap, error) {
	profiles := make(profileConfigMap, len(pConfig))

	for name, profConfig := range pConfig {
		if profConfig.DefinitionFile != "" {
			profDefinition, err := readProfileDefinition(profConfig.DefinitionFile)
			if err != nil {
				log.Warnf("failed to read profile definition `%s`: %s", name, err)
				continue
			}

			err = recursivelyExpandBaseProfiles(profConfig.DefinitionFile, profDefinition, profDefinition.Extends, []string{})
			if err != nil {
				log.Warnf("failed to expand profile `%s`: %s", name, err)
				continue
			}
			profConfig.Definition = *profDefinition
		}
		profiles[name] = profConfig
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
	profiledefinition.NormalizeMetrics(profileDefinition.Metrics)
	errors := validateEnrichMetadata(profileDefinition.Metadata)
	errors = append(errors, ValidateEnrichMetrics(profileDefinition.Metrics)...)
	errors = append(errors, ValidateEnrichMetricTags(profileDefinition.MetricTags)...)
	if len(errors) > 0 {
		return nil, fmt.Errorf("validation errors: %s", strings.Join(errors, "\n"))
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

func recursivelyExpandBaseProfiles(parentPath string, definition *profiledefinition.ProfileDefinition, extends []string, extendsHistory []string) error {
	parentBasePath := filepath.Base(parentPath)
	for _, extendEntry := range extends {
		// User profile can extend default profile by extending the default profile.
		// If the extend entry has the same name as the profile name, we assume the extend entry is referring to a default profile.
		if extendEntry == parentBasePath {
			extendEntry = filepath.Join(getProfileConfdRoot(defaultProfilesFolder), extendEntry)
		}
		for _, extend := range extendsHistory {
			if extend == extendEntry {
				return fmt.Errorf("cyclic profile extend detected, `%s` has already been extended, extendsHistory=`%v`", extendEntry, extendsHistory)
			}
		}
		baseDefinition, err := readProfileDefinition(extendEntry)
		if err != nil {
			return err
		}

		mergeProfileDefinition(definition, baseDefinition)

		newExtendsHistory := append(common.CopyStrings(extendsHistory), extendEntry)
		err = recursivelyExpandBaseProfiles(extendEntry, definition, baseDefinition.Extends, newExtendsHistory)
		if err != nil {
			return err
		}
	}
	return nil
}

func mergeProfileDefinition(targetDefinition *profiledefinition.ProfileDefinition, baseDefinition *profiledefinition.ProfileDefinition) {
	targetDefinition.Metrics = append(targetDefinition.Metrics, baseDefinition.Metrics...)
	targetDefinition.MetricTags = append(targetDefinition.MetricTags, baseDefinition.MetricTags...)
	targetDefinition.StaticTags = append(targetDefinition.StaticTags, baseDefinition.StaticTags...)
	for baseResName, baseResource := range baseDefinition.Metadata {
		if _, ok := targetDefinition.Metadata[baseResName]; !ok {
			targetDefinition.Metadata[baseResName] = profiledefinition.NewMetadataResourceConfig()
		}
		if resource, ok := targetDefinition.Metadata[baseResName]; ok {
			for _, tagConfig := range baseResource.IDTags {
				resource.IDTags = append(targetDefinition.Metadata[baseResName].IDTags, tagConfig)
			}

			if resource.Fields == nil {
				resource.Fields = make(map[string]profiledefinition.MetadataField, len(baseResource.Fields))
			}
			for field, symbol := range baseResource.Fields {
				if _, ok := resource.Fields[field]; !ok {
					resource.Fields[field] = symbol
				}
			}

			targetDefinition.Metadata[baseResName] = resource
		}
	}
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
