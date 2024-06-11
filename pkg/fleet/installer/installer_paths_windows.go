// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package installer

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/winregistry"
	"golang.org/x/sys/windows"
	"path/filepath"
)

var (
	// PackagesPath is the path to the packages directory.
	PackagesPath string

	// TmpDirPath is the path to the temporary directory used for package installation.
	TmpDirPath string

	// LocksPack is the path to the locks directory.
	LocksPack string

	// DefaultConfigsDir is the default Agent configuration directory
	DefaultConfigsDir string
)

func init() {
	PackagesPath, _ = winregistry.GetProgramDataDirForProduct("Datadog Installer")
	TmpDirPath = PackagesPath
	LocksPack = filepath.Join(PackagesPath, "locks")
	DefaultConfigsDir, _ = windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
}
