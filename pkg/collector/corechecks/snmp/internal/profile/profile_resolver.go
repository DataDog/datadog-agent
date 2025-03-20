// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"expvar"
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	profileExpVar = expvar.NewMap("snmpProfileErrors")
)

func resolveProfiles(userProfiles, defaultProfiles ProfileConfigMap) ProfileConfigMap {
	rawProfiles := mergeProfiles(defaultProfiles, userProfiles)
	userExpandedProfiles := normalizeProfiles(rawProfiles, defaultProfiles)
	return userExpandedProfiles
}

// normalizeProfiles returns a copy of pConfig with all profiles normalized, validated, and fully expanded (i.e. values from their .extend attributes will be baked into the profile itself).
func normalizeProfiles(pConfig ProfileConfigMap, defaultProfiles ProfileConfigMap) ProfileConfigMap {
	profiles := make(ProfileConfigMap, len(pConfig))

	for name := range pConfig {
		// No need to resolve abstract profile
		if strings.HasPrefix(name, "_") {
			continue
		}

		newProfileConfig := pConfig[name].Clone()
		err := recursivelyExpandBaseProfiles(name, &newProfileConfig.Definition, newProfileConfig.Definition.Extends, []string{}, pConfig, defaultProfiles)
		if err != nil {
			log.Warnf("failed to expand profile %q: %v", name, err)
			continue
		}
		profiledefinition.NormalizeMetrics(newProfileConfig.Definition.Metrics)
		errors := profiledefinition.ValidateEnrichMetadata(newProfileConfig.Definition.Metadata)
		errors = append(errors, profiledefinition.ValidateEnrichMetrics(newProfileConfig.Definition.Metrics)...)
		errors = append(errors, profiledefinition.ValidateEnrichMetricTags(newProfileConfig.Definition.MetricTags)...)
		if len(errors) > 0 {
			log.Warnf("validation errors in profile %q: %s", name, strings.Join(errors, "\n"))
			profileExpVar.Set(name, expvar.Func(func() interface{} {
				return strings.Join(errors, "\n")
			}))
			continue
		}
		profiles[name] = newProfileConfig
	}

	return profiles
}

func recursivelyExpandBaseProfiles(parentExtend string, definition *profiledefinition.ProfileDefinition, extends []string, extendsHistory []string, profiles ProfileConfigMap, defaultProfiles ProfileConfigMap) error {
	for _, extendEntry := range extends {
		extendEntry = strings.TrimSuffix(extendEntry, ".yaml")

		var baseDefinition *profiledefinition.ProfileDefinition
		// User profile can extend default profile by extending the default profile.
		// If the extend entry has the same name as the profile name, we assume the extend entry is referring to a default profile.
		if extendEntry == parentExtend {
			profile, ok := defaultProfiles[extendEntry]
			if !ok {
				return fmt.Errorf("extend does not exist: `%s`", extendEntry)
			}
			baseDefinition = &profile.Definition
		} else {
			profile, ok := profiles[extendEntry]
			if !ok {
				profile, ok = defaultProfiles[extendEntry]
				if !ok {
					return fmt.Errorf("extend does not exist: `%s`", extendEntry)
				}
			}
			baseDefinition = &profile.Definition
		}
		if slices.Contains(extendsHistory, extendEntry) {
			return fmt.Errorf("cyclic profile extend detected, `%s` has already been extended, extendsHistory=`%v`", extendEntry, extendsHistory)
		}

		mergeProfileDefinition(definition, baseDefinition)

		newExtendsHistory := append(utils.CopyStrings(extendsHistory), extendEntry)
		err := recursivelyExpandBaseProfiles(extendEntry, definition, baseDefinition.Extends, newExtendsHistory, profiles, defaultProfiles)
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
	if targetDefinition.Metadata == nil {
		targetDefinition.Metadata = make(profiledefinition.MetadataConfig)
	}
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
