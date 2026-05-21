// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides service manager utilities
package service

import "os/exec"

// Type is the service manager type
type Type string

const (
	// UnknownType is returned when the service manager type is not identified
	UnknownType Type = "unknown"
	// SysvinitType is returned when the service manager is sysvinit
	SysvinitType Type = "sysvinit"
	// UpstartType is returned when the service manager is upstart
	UpstartType Type = "upstart"
	// SystemdType is returned when the service manager is systemd
	SystemdType Type = "systemd"
	// ProcmgrType is systemd with the global procmgr gate open.
	ProcmgrType Type = "procmgr"
)

var cachedServiceManagerType *Type

// GetServiceManagerType returns the service manager of the current system.
func GetServiceManagerType() Type {
	if cachedServiceManagerType != nil {
		return *cachedServiceManagerType
	}
	serviceManagerType := getServiceManagerType()
	if serviceManagerType == SystemdType && procmgrEnabled() {
		serviceManagerType = ProcmgrType
	}
	cachedServiceManagerType = &serviceManagerType
	return serviceManagerType
}

// IsSystemdHost is true when systemctl exists (no procmgr gate, not cached).
func IsSystemdHost() bool {
	return getServiceManagerType() == SystemdType
}

func getServiceManagerType() Type {
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
