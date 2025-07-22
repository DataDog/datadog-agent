// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"maps"
	"slices"
)

// BuildProfile builds the fetchable profile for this config.
//
// If ProfileName == ProfileNameInline, then the result just contains the inline
// metrics and tags from the initconfig. This is also true if ProfileName ==
// ProfileNameAuto and sysObjectID == "" (this is useful when you want basic
// metadata for a device that you can't yet get the sysObjectID from).
//
// Otherwise, the result will be a copy of the profile from ProfileProvider that
// matches this config, either by sysObjectID if ProfileName == ProfileNameAuto
// or by ProfileName directly otherwise.
//
// The error will be non-nil if ProfileProvider doesn't know ProfileName, or if
// ProfileName is ProfileNameAuto and ProfileProvider finds no match for
// sysObjectID. In this case the returned profile will still be non-nil, and
// will be the same as what you'd get for an inline profile.
func (c *CheckConfig) BuildProfile(sysObjectID string) (profiledefinition.ProfileDefinition, error) {
	var rootProfile *profiledefinition.ProfileDefinition
	var profileErr error

	switch c.ProfileName {
	case ProfileNameInline: // inline profile -> no parent
		rootProfile = nil
	case ProfileNameAuto: // determine based on sysObjectID
		// empty sysObjectID happens when we need the profile but couldn't connect to the device.
		if sysObjectID != "" {
			if profileConfig, err := c.ProfileProvider.GetProfileForSysObjectID(sysObjectID); err != nil {
				profileErr = fmt.Errorf("failed to get profile for sysObjectID %q: %v", sysObjectID, err)
			} else {
				rootProfile = &profileConfig.Definition
				log.Debugf("detected profile %q for sysobjectid %q", rootProfile.Name, sysObjectID)
			}
		}
	default:
		if profile := c.ProfileProvider.GetProfile(c.ProfileName); profile == nil {
			profileErr = fmt.Errorf("unknown profile %q", c.ProfileName)
		} else {
			rootProfile = &profile.Definition
		}
	}

	profile := *profiledefinition.NewProfileDefinition()
	profile.Metrics = slices.Clone(c.RequestedMetrics)
	profile.MetricTags = slices.Clone(c.RequestedMetricTags)
	if rootProfile != nil {
		profile.Name = rootProfile.Name
		profile.Version = rootProfile.Version
		profile.StaticTags = append(profile.StaticTags, "snmp_profile:"+rootProfile.Name)
		vendor := rootProfile.Device.Vendor
		if vendor != "" {
			profile.StaticTags = append(profile.StaticTags, "device_vendor:"+vendor)
		}
		profile.StaticTags = append(profile.StaticTags, rootProfile.StaticTags...)
		profile.Metadata = maps.Clone(rootProfile.Metadata)
		profile.Metrics = append(profile.Metrics, rootProfile.Metrics...)
		profile.MetricTags = append(profile.MetricTags, rootProfile.MetricTags...)
		profile.Device.Vendor = rootProfile.Device.Vendor
	}
	profile.Metadata = updateMetadataDefinitionWithDefaults(profile.Metadata, c.CollectTopology, c.CollectVPN)

	return profile, profileErr
}
