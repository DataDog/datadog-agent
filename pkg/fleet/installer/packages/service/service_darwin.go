// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package service provides service manager utilities
package service

// Type is the service manager type
type Type string

const (
	// UnknownType is returned when the service manager type is not identified
	UnknownType Type = "unknown"
	// LaunchdType is returned when the service manager is launchd
	LaunchdType Type = "launchd"
)

// GetServiceManagerType returns the service manager of the current system.
//
// launchd is the only init system macOS has, and it is always present, so there is nothing to
// probe for: the Linux systemctl/initctl/update-rc.d probe would return UnknownType here.
func GetServiceManagerType(_ string) Type {
	return LaunchdType
}
