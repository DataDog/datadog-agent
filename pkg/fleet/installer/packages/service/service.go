// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides service manager utilities
package service

import (
	"os/exec"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
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
	// SystemdType is returned when the service manager is systemd
	SystemdType Type = "systemd"
	// ProcmgrType is returned when systemd is present and procmgr is installed + enabled
	// It will have its own logic, either delegated to systemd or managed by procmgr.
	ProcmgrType Type = "procmgr"
)

// initSystemType memoizes the init system probe. It never returns ProcmgrType: procmgr is a layer
// on top of systemd, not an init system.
var initSystemType = sync.OnceValue(detectInitSystem)

// procmgrEnabled and procmgrInstalled are indirected so tests can drive the selection.
var (
	procmgrEnabled   = func() bool { return env.FromEnv().ProcessManagerEnabled }
	procmgrInstalled = procmgr.IsInstalled
)

// GetServiceManagerType returns the service manager of the current system.
//
// procmgr is selected over plain systemd when the init system is systemd, the operator has not
// opted out via DD_PROCESS_MANAGER_ENABLED=false, and procmgr is actually installed. Only the init
// system probe is memoized: the other two change during an install, so they are re-evaluated on
// every call.
func GetServiceManagerType() Type {
	base := initSystemType()
	if base != SystemdType {
		return base
	}
	if !procmgrEnabled() || !procmgrInstalled() {
		return SystemdType
	}
	return ProcmgrType
}

func detectInitSystem() Type {
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
