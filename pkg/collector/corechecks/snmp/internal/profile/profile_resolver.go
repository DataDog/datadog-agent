// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/configvalidation"
)

func resolveProfiles(userProfiles, defaultProfiles ProfileConfigMap) (ProfileConfigMap, error) {
	rawProfiles := mergeProfiles(defaultProfiles, userProfiles)

	userExpandedProfiles, err := loadResolveProfiles(rawProfiles, defaultProfiles)
	if err != nil {
		return nil, fmt.Errorf("failed to load profiles: %s", err)
	}
	profiles := ProfileConfigMap{}

	for key, val := range userExpandedProfiles {
		if strings.HasPrefix(key, "_") {
			continue
		}
		profiles[key] = val
	}
	return profiles, nil
}

func loadResolveProfiles(pConfig ProfileConfigMap, defaultProfiles ProfileConfigMap) (ProfileConfigMap, error) {
	profiles := make(ProfileConfigMap, len(pConfig))

	for name, profConfig := range pConfig {
		err := recursivelyExpandBaseProfiles(name, &profConfig.Definition, profConfig.Definition.Extends, []string{}, pConfig, defaultProfiles)
		if err != nil {
			log.Warnf("failed to expand profile `%s`: %s", name, err)
			continue
		}
		profiledefinition.NormalizeMetrics(profConfig.Definition.Metrics)
		errors := configvalidation.ValidateEnrichMetadata(profConfig.Definition.Metadata)
		errors = append(errors, configvalidation.ValidateEnrichMetrics(profConfig.Definition.Metrics)...)
		errors = append(errors, configvalidation.ValidateEnrichMetricTags(profConfig.Definition.MetricTags)...)
		if len(errors) > 0 {
			log.Warnf("validation errors: %s", strings.Join(errors, "\n"))
			continue
		}
		profiles[name] = profConfig
	}

	return profiles, nil
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
		for _, extend := range extendsHistory {
			if extend == extendEntry {
				return fmt.Errorf("cyclic profile extend detected, `%s` has already been extended, extendsHistory=`%v`", extendEntry, extendsHistory)
			}
		}

		mergeProfileDefinition(definition, baseDefinition)

		newExtendsHistory := append(common.CopyStrings(extendsHistory), extendEntry)
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
