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
	"strings"
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
	// ProcmgrType is returned when systemd is present and the procmgrd
	// opt-in gate is open (DD_PROCMGR_MANAGE_DDOT=true or marker file).
	// Services with procmgrd fields use dd-procmgrd; others fall through
	// to systemd.
	ProcmgrType Type = "procmgr"

	procmgrDDOTMarkerPath = "/etc/datadog-agent/.procmgr-ddot-enabled"
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
	_, err := exec.LookPath("systemctl")
	if err == nil {
		if isProcmgrdGateOpen() {
			return ProcmgrType
		}
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

// isProcmgrdGateOpen returns true when the procmgrd DDOT opt-in gate is
// satisfied: either DD_PROCMGR_MANAGE_DDOT=true or the persistent marker
// file exists from a prior opt-in.
func isProcmgrdGateOpen() bool {
	if strings.EqualFold(os.Getenv("DD_PROCMGR_MANAGE_DDOT"), "true") {
		return true
	}
	_, err := os.Stat(procmgrDDOTMarkerPath)
	return err == nil
}
