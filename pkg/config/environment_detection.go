// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Feature represents a feature of current environment
type Feature string

const (
	autoconfEnvironmentVariable         = "AUTOCONFIG_FROM_ENVIRONMENT"
	autoconfEnvironmentVariableWithTypo = "AUTCONFIG_FROM_ENVIRONMENT"
)

// FeatureMap represents all detected features
type FeatureMap map[Feature]struct{}

func (fm FeatureMap) String() string {
	features := make([]string, 0, len(fm))
	for f := range fm {
		features = append(features, string(f))
	}

	return strings.Join(features, ",")
}

var (
	detectedFeatures FeatureMap
	featureLock      sync.RWMutex
)

// GetDetectedFeatures returns all detected features (detection only performed once)
func GetDetectedFeatures() FeatureMap {
	featureLock.RLock()
	defer featureLock.RUnlock()

	if detectedFeatures == nil {
		// If this function is called while feature detection has not run
		// it means Confifguration has not been loaded, which is an unexpected flow in our code
		// It's not useful to do lazy detection as it would also mean Configuration has not been loaded
		panic("Trying to access features before detection has run")
	}

	return detectedFeatures
}

// IsFeaturePresent returns if a particular feature is activated
func IsFeaturePresent(feature Feature) bool {
	featureLock.RLock()
	defer featureLock.RUnlock()

	if detectedFeatures == nil {
		// If this function is called while feature detection has not run
		// it means Confifguration has not been loaded, which is an unexpected flow in our code
		// It's not useful to do lazy detection as it would also mean Configuration has not been loaded
		panic("Trying to access features before detection has run")
	}

	_, found := detectedFeatures[feature]
	return found
}

// IsAutoconfigEnabled returns if autoconfig from environment is activated or not
func IsAutoconfigEnabled() bool {
	// Usage of pure environment variables should be deprecated
	for _, envVar := range []string{autoconfEnvironmentVariable, autoconfEnvironmentVariableWithTypo} {
		if autoconfStr, found := os.LookupEnv(envVar); found {
			activateAutoconfFromEnv, err := strconv.ParseBool(autoconfStr)
			if err != nil {
				log.Errorf("Unable to parse Autoconf value: '%s', err: %v - autoconfig from environment will be deactivated", autoconfStr, err)
				return false
			}

			log.Warnf("Usage of '%s' variable is deprecated - please use DD_AUTOCONFIG_FROM_ENVIRONMENT or 'autoconfig_from_environment' in config file", envVar)
			return activateAutoconfFromEnv
		}
	}

	return Datadog.GetBool("autoconfig_from_environment")
}

// We guarantee that Datadog configuration is entirely loaded (env + YAML)
// before this function is called
func detectFeatures() {
	featureLock.Lock()
	defer featureLock.Unlock()

	newFeatures := make(FeatureMap)
	if IsAutoconfigEnabled() {
		detectContainerFeatures(newFeatures)
		excludedFeatures := Datadog.GetStringSlice("autoconfig_exclude_features")
		for _, ef := range excludedFeatures {
			delete(newFeatures, Feature(ef))
		}

		log.Infof("Features detected from environment: %v", newFeatures)
	}
	detectedFeatures = newFeatures
}
