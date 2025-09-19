// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types holds types related to sbom
package types

// Package describes a system package
type Package struct {
	Name       string
	Version    string
	SrcVersion string
}

// PackageWithInstalledFiles describes a system package with its installed files
type PackageWithInstalledFiles struct {
	Package
	InstalledFiles []string
}
