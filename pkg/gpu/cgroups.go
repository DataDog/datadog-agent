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
	if err := configureSystemdDeviceAllow(systemdContainerID, rootfs); err != nil {
		return fmt.Errorf("failed to configure systemd device allow for container %s: %w", systemdContainerID, err)
	}

	// Configure cgroup device allow
	if err := configureCgroupDeviceAllow(string(cgroup.Path), rootfs); err != nil {
		return fmt.Errorf("failed to configure cgroup device allow for container %s: %w", cgroup.Path, err)
	}

	return nil
}

func configureSystemdDeviceAllow(containerID string, rootfs string) error {
	systemdDeviceAllowPath, err := buildSafePath(rootfs, "run/systemd/transient", containerID+".d", "50-DeviceAllow.conf")
	if err != nil {
		return fmt.Errorf("failed to build systemd/transient path: %w", err)
	}

	log.Debugf("configuring systemd device allow for container %s: %s", containerID, systemdDeviceAllowPath)

	// Open the systemd device allow file
	systemdDeviceAllow, err := os.OpenFile(systemdDeviceAllowPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", systemdDeviceAllowPath, err)
	}
	defer systemdDeviceAllow.Close()

	// Allow access to the NVIDIA character device.
	// Hardcode the string to avoid this from being accidentally changed to another value or set dynamically.
	_, err = systemdDeviceAllow.WriteString("DeviceAllow=char-nvidia rwm\n")
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", systemdDeviceAllowPath, err)
	}

	return nil
}

func configureCgroupDeviceAllow(containerID, rootfs string) error {
	cgroupDeviceAllowPath, err := buildSafePath(rootfs, "sys/fs/cgroup/devices", containerID, "devices.allow")
	if err != nil {
		return fmt.Errorf("failed to build cgroup/devices path: %w", err)
	}

	log.Debugf("configuring systemd device allow for container %s: %s", containerID, cgroupDeviceAllowPath)

	// Open the cgroup device allow file
	cgroupDeviceAllow, err := os.OpenFile(cgroupDeviceAllowPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", cgroupDeviceAllowPath, err)
	}
	defer cgroupDeviceAllow.Close()

	// Allow access to the NVIDIA character device. 195 is the major number for the NVIDIA character device.
	// Hardcode the string to avoid this from being accidentally changed to another value or set dynamically.
	_, err = cgroupDeviceAllow.WriteString("c 195:* rwm\n")
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", cgroupDeviceAllowPath, err)
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
