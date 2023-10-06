// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

// ProfileSource is the type used to indicate the source (e.g. custom or default) of the profile
type ProfileSource string

const (
	// SourceCustom enum for custom profile source
	SourceCustom ProfileSource = "custom"
	// SourceDefault enum for default profile source
	SourceDefault ProfileSource = "default"
)

// ProfileBundleProfileMetadata contains device profile metadata for downloaded profiles.
type ProfileBundleProfileMetadata struct {
	Source ProfileSource `json:"source"`
}

// ProfileBundleProfileItem represent a profile entry with metadata.
type ProfileBundleProfileItem struct {
	Metadata ProfileBundleProfileMetadata `json:"metadata"`
	Profile  ProfileDefinition            `json:"profile"`
}

// ProfileBundle represent a list of profiles meant to be downloaded by user.
type ProfileBundle struct {
	CreatedTimestamp int64                      `json:"created_timestamp"` // Millisecond
	Profiles         []ProfileBundleProfileItem `json:"profiles"`
}
