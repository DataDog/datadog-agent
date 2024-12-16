// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	var parentProfile *profiledefinition.ProfileDefinition
	var profileErr error

	switch c.ProfileName {
	case ProfileNameInline: // inline profile -> no parent
		parentProfile = nil
	case ProfileNameAuto: // determine based on sysObjectID
		// empty sysObjectID happens when we need the profile but couldn't connect to the device.
		if sysObjectID != "" {
			if profileConfig, err := c.ProfileProvider.GetProfileForSysObjectID(sysObjectID); err != nil {
				profileErr = fmt.Errorf("failed to get profile for sysObjectID %q: %v", sysObjectID, err)
			} else {
				parentProfile = &profileConfig.Definition
				log.Debugf("detected profile %q for sysobjectid %q", parentProfile.Name, sysObjectID)
			}
		}
	default:
		if profile := c.ProfileProvider.GetProfile(c.ProfileName); profile == nil {
			profileErr = fmt.Errorf("unknown profile %q", c.ProfileName)
		} else {
			parentProfile = &profile.Definition
		}
	}

	profile := *profiledefinition.NewProfileDefinition()
	profile.Metrics = slices.Clone(c.RequestedMetrics)
	profile.MetricTags = slices.Clone(c.RequestedMetricTags)
	if parentProfile != nil {
		profile.Name = parentProfile.Name
		profile.Version = parentProfile.Version
		profile.StaticTags = append(profile.StaticTags, "snmp_profile:"+parentProfile.Name)
		vendor := parentProfile.GetVendor()
		if vendor != "" {
			profile.StaticTags = append(profile.StaticTags, "device_vendor:"+vendor)
		}
		profile.StaticTags = append(profile.StaticTags, parentProfile.StaticTags...)
		profile.Metadata = parentProfile.Metadata
		profile.Metrics = append(profile.Metrics, parentProfile.Metrics...)
		profile.MetricTags = append(profile.MetricTags, parentProfile.MetricTags...)
	}
	profile.Metadata = updateMetadataDefinitionWithDefaults(profile.Metadata, c.CollectTopology)

	return profile, profileErr
}
