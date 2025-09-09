// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

package profile

// Cache holds the profile from previous successful runs of the device
type Cache struct {
	ProfileName   string
	Profile       *NCMProfile
	triedProfiles []string
}

// GetTriedProfiles lists names of profiles that had unsuccessful commands with the device
func (c *Cache) GetTriedProfiles() []string {
	return c.triedProfiles
}

// AppendToTriedProfiles allows to easily add profiles when iterating through profile options for a device
func (c *Cache) AppendToTriedProfiles(profile string) {
	c.triedProfiles = append(c.triedProfiles, profile)
}
