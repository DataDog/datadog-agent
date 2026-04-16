// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package types

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/acobaugh/osrelease"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// Backend is the minimum interface all kernel header backends must implement.
type Backend interface {
	GetKernelHeaders(directory string) error
	Close()
}

// Utsname are the kernel provided uname values
type Utsname struct {
	Kernel  string
	Machine string
}

// Distro is information about the host Linux distro
type Distro struct {
	Display string
	Release string
	Family  string
}

// Target is information about the host
type Target struct {
	Distro    Distro
	OSRelease map[string]string
	Uname     Utsname
}

// NewTarget collects and return the Target information about the host.
func NewTarget() (Target, error) {
	platform, err := kernel.Platform()
	if err != nil {
		return Target{}, fmt.Errorf("kernel platform: %w", err)
	}
	version, err := kernel.PlatformVersion()
	if err != nil {
		return Target{}, fmt.Errorf("kernel platform version: %w", err)
	}
	family, err := kernel.Family()
	if err != nil {
		return Target{}, fmt.Errorf("kernel family: %w", err)
	}

	target := Target{
		Distro: Distro{
			Display: platform,
			Release: version,
			Family:  family,
		},
	}

	r, err := kernel.Release()
	if err != nil {
		return target, fmt.Errorf("kernel release: %w", err)
	}
	target.Uname.Kernel = r

	m, err := kernel.Machine()
	if err != nil {
		return target, fmt.Errorf("kernel machine: %w", err)
	}
	target.Uname.Machine = m
	target.OSRelease = getOSRelease()

	if isWSL(target.Uname.Kernel) {
		target.Distro.Display, target.Distro.Family = "wsl", "wsl"
	} else if id := target.OSRelease["ID"]; target.Distro.Display == "" && id != "" {
		target.Distro.Display, target.Distro.Family = id, id
	}

	return target, nil
}

func isWSL(kernel string) bool {
	if strings.Contains(kernel, "Microsoft") {
		return true
	}
	if _, err := os.Stat("/run/WSL"); err == nil {
		return true
	}
	if f, err := os.ReadFile("/proc/version"); err == nil && strings.Contains(string(f), "Microsoft") {
		return true
	}
	return false
}

func getOSRelease() map[string]string {
	osReleasePaths := []string{
		HostEtc("os-release"),
		osrelease.UsrLibOsRelease,
	}

	var (
		release map[string]string
		err     error
	)
	for _, osReleasePath := range osReleasePaths {
		release, err = osrelease.ReadFile(osReleasePath)
		if err == nil {
			return release
		}
	}
	return make(map[string]string)
}

// Logger is the interface a logger must implement
type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// GetEnv retrieves the environment variable key. If it does not exist it returns the default.
func GetEnv(key string, dfault string, combineWith ...string) string {
	value := os.Getenv(key)
	if value == "" {
		value = dfault
	}

	switch len(combineWith) {
	case 0:
		return value
	case 1:
		return filepath.Join(value, combineWith[0])
	default:
		all := make([]string, len(combineWith)+1)
		all[0] = value
		copy(all[1:], combineWith)
		return filepath.Join(all...)
	}
}

// HostEtc joins the provided paths with `/etc` or the HOST_ETC env var value, if set.
func HostEtc(combineWith ...string) string {
	return GetEnv("HOST_ETC", "/etc", combineWith...)
}
