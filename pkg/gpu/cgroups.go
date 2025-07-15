// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"

	"github.com/containerd/cgroups/v3"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/cgroupns"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigureDeviceCgroups configures the cgroups for a process to allow access to the NVIDIA character devices
func ConfigureDeviceCgroups(pid uint32, rootfs string) error {
	cgroupPath, err := getCgroupForProcess(filepath.Join(rootfs, "proc"), pid)
	if err != nil {
		return fmt.Errorf("failed to get cgroup for pid %d: %w", pid, err)
	}

	// Configure systemd device allow first, so that in case of a reload we get the correct permissions
	// The containerID for systemd is the last part of the cgroup path
	if err := configureSystemDAllow(filepath.Base(cgroupPath), rootfs); err != nil {
		return fmt.Errorf("failed to configure systemd device allow for cgroup %s: %w", cgroupPath, err)
	}

	// Now configure the cgroup device allow, depending on the cgroup version
	if cgroups.Mode() == cgroups.Legacy {
		err = configureCgroupV1DeviceAllow(cgroupPath, rootfs, nvidiaDeviceMajor)
	} else {
		err = detachAllDeviceCgroupPrograms(cgroupPath, rootfs)
	}

	if err != nil {
		return fmt.Errorf("failed to configure cgroup device allow for cgroup path %s: %w", cgroupPath, err)
	}

	return nil
}

const (
	systemdDeviceAllowFile     = "50-DeviceAllow.conf"
	systemdTransientConfigPath = "run/systemd/transient"
	cgroupv1DeviceAllowFile    = "devices.allow"
	cgroupv1DeviceAllowDir     = "sys/fs/cgroup/devices"
	nvidiaSystemdDeviceAllow   = "DeviceAllow=char-nvidia rwm\n" // Allow access to the NVIDIA character devices
	nvidiaDeviceMajor          = 195
)

// getCgroupForProcess gets the cgroup path for a process independently of whether
// we are inside a container or not, or of the cgroup version being used.
// Requires CAP_SYS_ADMIN to work.
func getCgroupForProcess(procRoot string, pid uint32) (string, error) {
	var cgroupPath string

	// We need to enter the root cgroup namespace to get the correct cgroup
	// path, If we don't and we are inside a container with cgroups v2, we will
	// only get the cgroup name from the container namespace, which usually will just be "/"
	err := cgroupns.WithRootNS(procRoot, func() error {
		cgroups, err := utils.GetProcControlGroups(pid, pid)
		if err != nil {
			return fmt.Errorf("failed to get cgroups for pid %d: %w", pid, err)
		}

		if len(cgroups) == 0 {
			return fmt.Errorf("no cgroups found for pid %d", pid)
		}

		// Each cgroup is for a different subsystem, we only want the cgroup ID
		// and we can extract that from any cgroup
		cgroupPath = string(cgroups[0].Path)
		return nil
	})

	return cgroupPath, err
}

// insertAfterSection finds a section header in the lines and inserts the new line after it
func insertAfterSection(lines []string, sectionHeader, newLine string) ([]string, error) {
	// Find the section header line
	sectionIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == sectionHeader {
			sectionIndex = i
			break
		}
	}

	if sectionIndex == -1 {
		return nil, fmt.Errorf("failed to find section header %s", sectionHeader)
	}

	// Insert the new line after the section header
	newLines := make([]string, len(lines)+1)
	copy(newLines, lines[:sectionIndex+1])
	newLines[sectionIndex+1] = newLine
	copy(newLines[sectionIndex+2:], lines[sectionIndex+1:])

	return newLines, nil
}

func configureSystemDAllow(containerID, rootfs string) error {
	// The SystemD device configuration might be either in a 50-DeviceAllow.conf file
	// in a service configuration directory, or in a service file directly. Default to the .conf
	// file and fall back to the service file if it doesn't exist.
	configFilePath, err := buildSafePath(rootfs, systemdTransientConfigPath, containerID+".d", systemdDeviceAllowFile)
	if err != nil {
		return fmt.Errorf("failed to build path for systemd device allow: %w", err)
	}

	if _, err := os.Stat(configFilePath); errors.Is(err, os.ErrNotExist) {
		configFilePath, err := buildSafePath(rootfs, systemdTransientConfigPath, containerID, ".service")
		if err != nil {
			return fmt.Errorf("failed to build path for systemd service file: %w", err)
		}

		if _, err := os.Stat(configFilePath); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to find either a SystemD configuration directory or configuration file for container %s", containerID)
		}
	}

	// Read the entire file
	content, err := os.ReadFile(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configFilePath, err)
	}

	lines := strings.Split(string(content), "\n")

	// Insert the nvidiaDeviceAllow line after [Service]
	newLines, err := insertAfterSection(lines, "[Scope]", nvidiaSystemdDeviceAllow)
	if err != nil {
		return fmt.Errorf("failed to insert device allow line in %s: %w", configFilePath, err)
	}

	// Write the modified content back to the file
	newContent := strings.Join(newLines, "\n")
	err = os.WriteFile(configFilePath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write modified config to %s: %w", configFilePath, err)
	}

	return nil
}

func configureCgroupV1DeviceAllow(rootfs, cgroupPath string, majorNumber int) error {
	deviceAllowPath, err := buildSafePath(rootfs, cgroupv1DeviceAllowDir, cgroupPath, cgroupv1DeviceAllowFile)
	if err != nil {
		return fmt.Errorf("failed to build path for cgroupv1 device allow: %w", err)
	}

	log.Debugf("configuring cgroupv1 device allow for cgroup path %s: %s", cgroupPath, deviceAllowPath)

	deviceAllowFile, err := os.OpenFile(deviceAllowPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", deviceAllowPath, err)
	}
	defer deviceAllowFile.Close()

	cgroupAllowLine := fmt.Sprintf("c %d:* rwm\n", majorNumber)
	_, err = deviceAllowFile.WriteString(cgroupAllowLine)
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", deviceAllowPath, err)
	}

	return nil
}

// detachAllDeviceCgroupPrograms finds and detaches all device cgroup BPF programs from a cgroup
// cgroupName is the name of the cgroup, e.g. "kubepods.slice"
// rootfs is the rootfs where /sys/fs/cgroup is mounted
func detachAllDeviceCgroupPrograms(cgroupName, rootfs string) error {
	cgroupHostPath, err := buildSafePath(rootfs, "sys/fs/cgroup", cgroupName)
	if err != nil {
		return fmt.Errorf("failed to build host path for cgroup %s: %w", cgroupName, err)
	}

	cgroup, err := os.Open(cgroupHostPath)
	if err != nil {
		return fmt.Errorf("failed to open cgroup %s: %w", cgroupHostPath, err)
	}
	defer cgroup.Close()

	// Query for all attached device programs
	queryResult, err := link.QueryPrograms(link.QueryOptions{
		Target: int(cgroup.Fd()),
		Attach: ebpf.AttachCGroupDevice,
	})
	if err != nil {
		return fmt.Errorf("failed to query device programs for cgroup %s: %w", cgroupName, err)
	}

	if len(queryResult.Programs) == 0 {
		log.Debugf("no device programs found attached to cgroup %s", cgroupName)
		return nil
	}

	log.Debugf("found %d device programs attached to cgroup %s", len(queryResult.Programs), cgroupName)

	// Detach each program
	var detachErrs error
	for _, prog := range queryResult.Programs {
		// Load the program by ID
		program, err := ebpf.NewProgramFromID(prog.ID)
		if err != nil {
			detachErrs = errors.Join(detachErrs, fmt.Errorf("failed to load program %d for cgroup %s: %w", prog.ID, cgroupName, err))
			continue
		}

		// Detach the program
		err = link.RawDetachProgram(link.RawDetachProgramOptions{
			Target:  int(cgroup.Fd()),
			Program: program,
			Attach:  ebpf.AttachCGroupDevice,
		})
		if err != nil {
			program.Close()
			detachErrs = errors.Join(detachErrs, fmt.Errorf("failed to detach program %d from cgroup %s: %w", prog.ID, cgroupName, err))
			continue
		}

		log.Debugf("successfully detached device program %d from cgroup %s", prog.ID, cgroupName)
		program.Close()
	}

	return detachErrs
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
