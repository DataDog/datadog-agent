// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winsoftware

import (
	"fmt"
)

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

// Source identifiers for software entries
const (
	// sourceRegistry indicates the software was found in the Windows Registry
	sourceRegistry = "registry"
	// sourceMSI indicates the software was found via MSI API
	sourceMSI = "msi"

	// Common properties for all sources
	// installDate is the installation date in YYYYMMDD format
	installDate = "InstallDate"
	// publisher is the software publisher/vendor
	publisher = "Publisher"
	// versionMinor is the minor version number
	versionMinor = "VersionMinor"
	// versionMajor is the major version number
	versionMajor = "VersionMajor"
)

// Warning represents a non-fatal error during collection
type Warning struct {
	Message string
}

func warnf(format string, args ...interface{}) *Warning {
	return &Warning{Message: fmt.Sprintf(format, args...)}
}

// SoftwareEntry represents a software installation
type SoftwareEntry struct {
	DisplayName string            `json:"display_name"`
	Version     string            `json:"version"`
	InstallDate string            `json:"install_date,omitempty"`
	Source      string            `json:"source"`
	UserSID     string            `json:"user_sid,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
	Is64Bit     bool              `json:"is_64_bit"`
}

// GetID returns a unique identifier for the software entry
func (se *SoftwareEntry) GetID() string {
	return se.DisplayName
}
