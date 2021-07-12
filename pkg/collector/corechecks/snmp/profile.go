package snmp

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type profileDefinitionMap map[string]profileDefinition

type deviceMeta struct {
	Vendor string `yaml:"vendor"`
}

type profileDefinition struct {
	Metrics      []metricsConfig   `yaml:"metrics"`
	MetricTags   []metricTagConfig `yaml:"metric_tags"`
	Extends      []string          `yaml:"extends"`
	Device       deviceMeta        `yaml:"device"`
	SysObjectIds StringArray       `yaml:"sysobjectid"`
}

var defaultProfilesMu = &sync.Mutex{}
var globalProfileConfigMap profileDefinitionMap

// loadDefaultProfiles will load the profiles from disk only once and store it
// in globalProfileConfigMap. The subsequent call to it will return profiles stored in
// globalProfileConfigMap. The mutex will help loading once when `loadDefaultProfiles`
// is called by multiple check instances.
func loadDefaultProfiles() (profileDefinitionMap, error) {
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
	profilesRoot := getProfileConfdRoot()
	files, err := ioutil.ReadDir(profilesRoot)
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
		profiles[profileName] = profileConfig{filepath.Join(profilesRoot, fName)}
	}
	return profiles, nil
}

func loadProfiles(pConfig profileConfigMap) (profileDefinitionMap, error) {
	profiles := make(map[string]profileDefinition, len(pConfig))

	for name, profile := range pConfig {
		definitionFile := profile.DefinitionFile

		profileDefinition, err := readProfileDefinition(definitionFile)
		if err != nil {
			log.Warnf("failed to read profile definition `%s`: %s", name, err)
			continue
		}

		err = recursivelyExpandBaseProfiles(profileDefinition, profileDefinition.Extends, []string{})
		if err != nil {
			log.Warnf("failed to expand profile `%s`: %s", name, err)
			continue
		}

		profiles[name] = *profileDefinition
	}
	return profiles, nil
}

func readProfileDefinition(definitionFile string) (*profileDefinition, error) {
	filePath := resolveProfileDefinitionPath(definitionFile)
	buf, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file `%s`: %s", filePath, err)
	}

	profileDefinition := &profileDefinition{}
	err = yaml.Unmarshal(buf, profileDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall %q: %v", filePath, err)
	}
	normalizeMetrics(profileDefinition.Metrics)
	errors := validateEnrichMetrics(profileDefinition.Metrics)
	errors = append(errors, validateEnrichMetricTags(profileDefinition.MetricTags)...)
	if len(errors) > 0 {
		return nil, fmt.Errorf("validation errors: %s", strings.Join(errors, "\n"))
	}
	return profileDefinition, nil
}

func resolveProfileDefinitionPath(definitionFile string) string {
	if filepath.IsAbs(definitionFile) {
		return definitionFile
	}
	return filepath.Join(getProfileConfdRoot(), definitionFile)
}

func getProfileConfdRoot() string {
	confdPath := config.Datadog.GetString("confd_path")
	return filepath.Join(confdPath, "snmp.d", "profiles")
}

func recursivelyExpandBaseProfiles(definition *profileDefinition, extends []string, extendsHistory []string) error {
	for _, basePath := range extends {
		for _, extend := range extendsHistory {
			if extend == basePath {
				return fmt.Errorf("cyclic profile extend detected, `%s` has already been extended, extendsHistory=`%v`", basePath, extendsHistory)
			}
		}
		baseDefinition, err := readProfileDefinition(basePath)
		if err != nil {
			return err
		}
		definition.Metrics = append(definition.Metrics, baseDefinition.Metrics...)
		definition.MetricTags = append(definition.MetricTags, baseDefinition.MetricTags...)

		newExtendsHistory := append(copyStrings(extendsHistory), basePath)
		err = recursivelyExpandBaseProfiles(definition, baseDefinition.Extends, newExtendsHistory)
		if err != nil {
			return err
		}
	}
	return nil
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
