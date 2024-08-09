// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package paths defines commonly used paths throughout the installer
package paths

const (
	// PackagesPath is the path to the packages directory.
	PackagesPath = "/opt/datadog-packages"
	// ConfigsPath is the path to the Fleet-managed configuration directory.
	ConfigsPath = "/etc/datadog-packages"
	// LocksPath is the path to the packages locks directory.
	LocksPath = "/var/run/datadog-installer/locks"

	// DefaultUserConfigsDir is the default Agent configuration directory.
	DefaultUserConfigsDir = "/etc"
)
