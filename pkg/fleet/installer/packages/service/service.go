// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides service manager utilities
package service

import (
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/procmgr"
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
	// SystemdType is returned when the service manager is systemd and the
	// global process-manager gate is closed (see ProcmgrType).
	SystemdType Type = "systemd"
	// ProcmgrType is returned when systemd is present and the global procmgr
	// gate is open (see procmgr.GlobalGateOpen and procmgr package constants).
	ProcmgrType Type = "procmgr"
)

var cachedBaseServiceManagerType *Type

// BaseServiceManagerType returns systemd / upstart / sysvinit / unknown without
// applying the process-manager gate. It is cached for the process lifetime.
func BaseServiceManagerType() Type {
	if cachedBaseServiceManagerType != nil {
		return *cachedBaseServiceManagerType
	}
	t := baseServiceManagerType()
	cachedBaseServiceManagerType = &t
	return t
}

// baseServiceManagerType detects systemd vs upstart vs sysvinit (no procmgr gate).
func baseServiceManagerType() Type {
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

// GetServiceManagerType returns the effective service manager for the host.
// When the base is systemd and the global procmgr gate is open, this returns
// ProcmgrType; otherwise it returns the base type. Only BaseServiceManagerType
// is cached; the gate is re-read each call (cheap) so marker/env match disk
// after hooks run.
func GetServiceManagerType() Type {
	base := BaseServiceManagerType()
	if base == SystemdType && procmgr.GlobalGateOpen() {
		return ProcmgrType
	}
	return base
}
