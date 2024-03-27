// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// loadBundleJSONProfiles finds the gzipped profile bundle and loads profiles from it.
func loadBundleJSONProfiles(gzipFilePath string) (ProfileConfigMap, error) {
	jsonStr, err := loadGzipFile(gzipFilePath)
	if err != nil {
		return nil, err
	}

	userProfiles, err := unmarshallProfilesBundleJSON(jsonStr, gzipFilePath)
	if err != nil {
		return nil, err
	}
	// TODO (separate PR): Use default profiles from json Bundle in priority once it's implemented.
	//       We fallback on Yaml Default Profiles if default profiles are not present in json Bundle.
	defaultProfiles := getYamlDefaultProfiles()

	resolvedProfiles, err := resolveProfiles(userProfiles, defaultProfiles)
	if err != nil {
		return nil, err
	}

	return resolvedProfiles, nil
}

// unmarshallProfilesBundleJSON parses json data into a profile bundle.
// Duplicate profiles and profiles without names will be skipped, and warnings will be logged;
// filenameForLogging is only used for making more readable log messages.
func unmarshallProfilesBundleJSON(raw []byte, filenameForLogging string) (ProfileConfigMap, error) {
	bundle := profiledefinition.ProfileBundle{}
	err := json.Unmarshal(raw, &bundle)
	if err != nil {
		return nil, err
	}

	profiles := make(ProfileConfigMap)
	for i, p := range bundle.Profiles {
		if p.Profile.Name == "" {
			log.Warnf("ignoring profile #%d from %q - no name provided", i, filenameForLogging)
			continue
		}

		if _, exist := profiles[p.Profile.Name]; exist {
			log.Warnf("ignoring duplicate profile in %q for name %q", filenameForLogging, p.Profile.Name)
			continue
		}
		// TODO: (separate PR) resolve extends with custom + local default profiles (yaml)
		profiles[p.Profile.Name] = ProfileConfig{
			Definition:    p.Profile,
			IsUserProfile: true,
		}
	}
	return profiles, nil
}

// loadGzipFile extracts the contents of a gzip file.
func loadGzipFile(filePath string) ([]byte, error) {
	gzipFile, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer gzipFile.Close()
	gzipReader, err := gzip.NewReader(gzipFile)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()
	return io.ReadAll(gzipReader)
}

// getProfileBundleFilePath returns the expected location of the gzipped profiles bundle, based on config.Datadog.
func getProfileBundleFilePath() string {
	return getProfileConfdRoot(filepath.Join(userProfilesFolder, profilesJSONGzipFile))
}

// findProfileBundleFilePath returns the path to the gzipped profiles bundle, or "" if one doesn't exist.
func findProfileBundleFilePath() string {
	filePath := getProfileBundleFilePath()
	if pathExists(filePath) {
		return filePath
	}
	return ""
}
