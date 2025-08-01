// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

// Installation context flags from Windows SDK (msi.h)
// See: https://learn.microsoft.com/en-us/windows/win32/msi/product-context
const (
	//nolint:revive // Keep these constants in sync with the Windows SDK
	MSIINSTALLCONTEXT_USERMANAGED = 1
	//nolint:revive // Keep these constants in sync with the Windows SDK
	MSIINSTALLCONTEXT_USERUNMANAGED = 2
	//nolint:revive // Keep these constants in sync with the Windows SDK
	MSIINSTALLCONTEXT_MACHINE = 4
	//nolint:revive // Keep these constants in sync with the Windows SDK
	MSIINSTALLCONTEXT_ALL = 7
)

// Software types represent the different sources of software installations
const (
	// softwareTypeDesktop represents traditional desktop applications
	softwareTypeDesktop = "desktop"
	// softwareTypeMSStore represents Microsoft Store applications
	//nolint:unused // Not implemented yet
	softwareTypeMSStore = "msstore"
	// softwareTypeMSU represents Microsoft Update standalone packages
	//nolint:unused // Not implemented yet
	softwareTypeMSU = "msu"
)

// Common properties for all sources
const (
	// installDate is the installation date in YYYYMMDD format
	installDate = "InstallDate"
	// publisher is the software publisher/vendor
	publisher = "Publisher"
	// versionMinor is the minor version number
	versionMinor = "VersionMinor"
	// versionMajor is the major version number
	versionMajor = "VersionMajor"
)
