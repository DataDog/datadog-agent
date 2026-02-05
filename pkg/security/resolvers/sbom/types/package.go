// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types holds types related to sbom
package types

import "time"

// Package describes a system package
type Package struct {
	Name       string
	Version    string
	Epoch      int
	Release    string
	SrcVersion string
	SrcEpoch   int
	SrcRelease string
	LastAccess time.Time
}

// PackageWithInstalledFiles describes a system package with its installed files
type PackageWithInstalledFiles struct {
	Package
	InstalledFiles []string
}
