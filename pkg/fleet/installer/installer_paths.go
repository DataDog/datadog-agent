// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package installer

const (
	// PackagesPath is the path to the packages directory.
	PackagesPath = "/opt/datadog-packages"
	// TmpDirPath is the path to the temporary directory used for package installation.
	TmpDirPath = "/opt/datadog-packages"
	// LocksPack is the path to the locks directory.
	LocksPack = "/var/run/datadog-packages"
	// DefaultConfigsDir is the default Agent configuration directory
	DefaultConfigsDir = "/etc"
)
