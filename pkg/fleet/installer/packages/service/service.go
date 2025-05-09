// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import "os/exec"

// Type is the service manager type
type Type int

const (
	// UnknownType is returned when the service manager type is not identified
	UnknownType Type = iota
	// SysvinitType is returned when the service manager is sysvinit
	SysvinitType
	// UpstartType is returned when the service manager is upstart
	UpstartType
	// SystemdType is returned when the service manager is systemd
	SystemdType
)

// GetServiceManagerType returns the service manager of the current system
func GetServiceManagerType() Type {
	_, err := exec.LookPath("systemctl")
	if err == nil {
		return SystemdType
	}
	_, err = exec.LookPath("initctl")
	if err == nil {
		return UpstartType
	}
	_, err = exec.LookPath("update-rc.d")
	if err == nil {
		return SysvinitType
	}
	return UnknownType
}
