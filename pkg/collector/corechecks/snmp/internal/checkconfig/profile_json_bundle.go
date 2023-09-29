package checkconfig

import (
	"compress/gzip"
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"io"
	"os"
	"path/filepath"
)

func loadBundleZipProfiles() (profileConfigMap, error) {
	jsonStr, err := getProfilesBundleJson()
	if err != nil {
		return nil, err
	}

	profiles, err := unmarshallProfilesBundleJson(jsonStr)
	if err != nil {
		return nil, err
	}
	return profiles, nil
}

func unmarshallProfilesBundleJson(jsonStr []byte) (profileConfigMap, error) {
	bundle := profiledefinition.ProfileBundle{}
	err := json.Unmarshal(jsonStr, &bundle)
	if err != nil {
		return nil, err
	}

	profiles := make(profileConfigMap)
	for _, profile := range bundle.Profiles {
		if profile.Profile.Name == "" {
			log.Warnf("Profile with missing name: %s", profile.Profile.Name)
			continue
		}

		if _, exist := profiles[profile.Profile.Name]; exist {
			log.Warnf("duplicate profile found: %s", profile.Profile.Name)
			continue
		}
		isUserProfile := profile.Metadata.Source == profiledefinition.SourceCustom
		profiles[profile.Profile.Name] = profileConfig{
			Definition:    profile.Profile,
			isUserProfile: isUserProfile,
		}
	}
	return profiles, nil
}

func getProfilesBundleJson() ([]byte, error) {
	gzipFilePath := getGZipFilePath()
	gzipFile, err := os.Open(gzipFilePath)
	if err != nil {
		return nil, err
	}
	defer gzipFile.Close()
	gzipReader, err := gzip.NewReader(gzipFile)
	if err != nil {
		return nil, err
	}
	jsonStr, err := io.ReadAll(gzipReader)
	if err != nil {
		return nil, err
	}
	return jsonStr, nil
}

func getGZipFilePath() string {
	return getProfileConfdRoot(filepath.Join(userProfilesFolder, profilesJsonGzipFile))
}
