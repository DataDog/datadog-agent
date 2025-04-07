// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigureDeviceCgroups configures the cgroups for a process to allow access to the NVIDIA character devices
func ConfigureDeviceCgroups(pid uint32, rootfs string) error {
	cgroups, err := utils.GetProcControlGroups(pid, pid)
	if err != nil {
		return fmt.Errorf("failed to get cgroups for pid %d: %w", pid, err)
	}

	if len(cgroups) == 0 {
		return fmt.Errorf("no cgroups found for pid %d", pid)
	}

	// Each cgroup is for a different subsystem, we only want the cgroup ID
	// and we can extract that from any cgroup
	cgroup := cgroups[0]

	// Configure systemd device allow first, so that in case of a reload we get the correct permissions
	// The containerID for systemd is the last part of the cgroup path
	systemdContainerID := filepath.Base(string(cgroup.Path))
	if err := configureDeviceAllow(systemdContainerID, rootfs, systemdDev); err != nil {
		return fmt.Errorf("failed to configure systemd device allow for container %s: %w", systemdContainerID, err)
	}

	// Configure cgroup device allow
	if err := configureDeviceAllow(string(cgroup.Path), rootfs, cgroupDev); err != nil {
		return fmt.Errorf("failed to configure cgroup device allow for container %s: %w", cgroup.Path, err)
	}

	return nil
}

const (
	systemdDeviceAllowFile = "50-DeviceAllow.conf"
	systemdDeviceAllowDir  = "run/systemd/transient"
	cgroupDeviceAllowFile  = "devices.allow"
	cgroupDeviceAllowDir   = "sys/fs/cgroup/devices"
	nvidiaDeviceAllow      = "DeviceAllow=char-nvidia rwm\n" // Allow access to the NVIDIA character devices
	nvidiaCgroupAllow      = "c 195:* rwm\n"                 // 195 is the major number for the NVIDIA character devices
)

type deviceType string

const (
	systemdDev deviceType = "systemd"
	cgroupDev  deviceType = "cgroup"
)

func configureDeviceAllow(containerID, rootfs string, devType deviceType) error {
	var deviceAllowPath string
	var err error
	var allowString string

	switch devType {
	case systemdDev:
		deviceAllowPath, err = buildSafePath(rootfs, systemdDeviceAllowDir, containerID+".d", systemdDeviceAllowFile)
		allowString = nvidiaDeviceAllow
	case cgroupDev:
		deviceAllowPath, err = buildSafePath(rootfs, cgroupDeviceAllowDir, containerID, cgroupDeviceAllowFile)
		allowString = nvidiaCgroupAllow
	default:
		return fmt.Errorf("unknown device type: %s", devType)
	}

	if err != nil {
		return fmt.Errorf("failed to build path for %s: %w", devType, err)
	}

	log.Debugf("configuring %s device allow for container %s: %s", devType, containerID, deviceAllowPath)

	deviceAllowFile, err := os.OpenFile(deviceAllowPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", deviceAllowPath, err)
	}
	defer deviceAllowFile.Close()

	_, err = deviceAllowFile.WriteString(allowString)
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", deviceAllowPath, err)
	}

	return nil
}

// buildSafePath builds a safe path from the rootfs and basedir, and appends the
// parts to it. It assumes that rootfs and basedir are already validated paths,
// and check that the parts being added to the path do not cause the final path
// to escape the rootfs/basedir.
func buildSafePath(rootfs string, basedir string, parts ...string) (string, error) {
	rootfs = strings.TrimSuffix(rootfs, "/")   // Remove trailing slashes from rootfs
	basedir = strings.TrimPrefix(basedir, "/") // Remove leading slashes from basedir

	// that way we can now join the paths using Sprintf to build the base directory
	root := fmt.Sprintf("%s/%s", rootfs, basedir)

	// Join the parts to the base directory and create a full path. Note that this will also remove any ".." from the path
	fullPath := filepath.Join(append([]string{root}, parts...)...)

	// Check that the resulting path is a child of root and that we haven't escaped the rootfs/basedir
	if !strings.HasPrefix(fullPath, root) {
		return "", fmt.Errorf("invalid path %s, should be a child of %s", fullPath, root)
	}

	return fullPath, nil
}
