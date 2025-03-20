// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import "strings"

// PackageVersion is a helper type to store both the version and the package version of a binary.
// The package version has the "-1" suffix, whereas the binary's "version" command does not contain the "-1" suffix.
type PackageVersion struct {
	value        string
	packageValue string
}

// Version the version without the package suffix
func (v PackageVersion) Version() string {
	return v.value
}

// PackageVersion the version with the package suffix
func (v PackageVersion) PackageVersion() string {
	return v.packageValue
}

// NewVersionFromPackageVersion creates a new PackageVersion from a package version string
func NewVersionFromPackageVersion(packageVersion string) PackageVersion {
	return PackageVersion{
		value:        strings.TrimSuffix(packageVersion, "-1"),
		packageValue: packageVersion,
	}
}

// PackageEntry is a struct to represent a package entry in the catalog
type PackageEntry struct {
	Package string `json:"package"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Catalog is a struct to represent a catalog of packages, used with the set-catalog subcommand
type Catalog struct {
	Packages []PackageEntry `json:"packages"`
}
