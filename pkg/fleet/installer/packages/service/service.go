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
	// ContainerType is returned when running in a container without init system
	ContainerType Type = "container"
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
	// Check if systemd is actually running (not just installed)
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return SystemdType
	}

	// Check for upstart
	_, err := exec.LookPath("initctl")
	if err == nil {
		return UpstartType
	}

	// Check for sysvinit
	_, err = exec.LookPath("update-rc.d")
	if err == nil {
		return SysvinitType
	}

	// Check if running in a containerized environment without init system
	// Check for DOCKER_DD_AGENT env var (set in official Datadog Docker images)
	if os.Getenv("DOCKER_DD_AGENT") != "" {
		return ContainerType
	}
	// Check for /.dockerenv file (typically only set by Docker)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return ContainerType
	}

	return UnknownType
}
