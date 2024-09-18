// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package paths defines commonly used paths throughout the installer
package paths

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/winregistry"
	"golang.org/x/sys/windows"
)

var (
	// PackagesPath is the path to the packages directory.
	PackagesPath string
	// ConfigsPath is the path to the Fleet-managed configuration directory
	ConfigsPath string
	// LocksPath is the path to the locks directory.
	LocksPath string
	// RootTmpDir is the temporary path where the bootstrapper will be extracted to.
	RootTmpDir string
	// DefaultUserConfigsDir is the default Agent configuration directory
	DefaultUserConfigsDir string
	// StableInstallerPath is the path to the stable installer binary.
	StableInstallerPath string
)

func init() {
	datadogInstallerData, _ := winregistry.GetProgramDataDirForProduct("Datadog Installer")
	PackagesPath = filepath.Join(datadogInstallerData, "packages")
	ConfigsPath = filepath.Join(datadogInstallerData, "configs")
	LocksPath = filepath.Join(datadogInstallerData, "locks")
	RootTmpDir = filepath.Join(datadogInstallerData, "tmp")
	datadogInstallerPath := "C:\\Program Files\\Datadog\\Datadog Installer"
	StableInstallerPath = filepath.Join(datadogInstallerPath, "datadog-installer.exe")
	DefaultUserConfigsDir, _ = windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
}
