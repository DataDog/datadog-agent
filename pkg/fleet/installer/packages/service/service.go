// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides service manager utilities
package service

import (
	"os"
	"os/exec"
)

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
)

var cachedServiceManagerType *Type

// GetServiceManagerType returns the service manager of the current system
func GetServiceManagerType() Type {
	if cachedServiceManagerType != nil {
		return *cachedServiceManagerType
	}
	serviceManagerType := getServiceManagerType()
	cachedServiceManagerType = &serviceManagerType
	return serviceManagerType
}

func getServiceManagerType() Type {
	_, err := os.Stat("/run/systemd/system")
	if err == nil {
		return SystemdType
	}
	if _, err := os.Stat("/etc/rc.d"); err == nil {
		return SysvinitType
	}
	if _, err := os.Stat("/etc/init.d"); err == nil {
		return SysvinitType
	}
	_, err = exec.LookPath("initctl")
	if err == nil {
		return UpstartType
	}
	return UnknownType
}
