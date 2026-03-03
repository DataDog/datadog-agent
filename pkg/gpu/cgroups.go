// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"

	"github.com/containerd/cgroups/v3"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigureDeviceCgroups configures the cgroups for a process to allow access to the NVIDIA character devices
// reapplyInfinitely controls whether the configuration should be reapplied infinitely (true) or only once (false)
func ConfigureDeviceCgroups(pid uint32, hostRoot string) error {
	cgroupMode := cgroups.Mode()
	cgroupPath, err := getAbsoluteCgroupForProcess("/", hostRoot, uint32(os.Getpid()), pid, cgroupMode)
	if err != nil {
		return fmt.Errorf("failed to get cgroup for pid %d: %w", pid, err)
	}

	// Configure systemd device allow first, so that in case of a reload we get the correct permissions
	// The containerID for systemd is the last part of the cgroup path
	if err := configureSystemDAllow(hostRoot, filepath.Base(cgroupPath)); err != nil {
		return fmt.Errorf("failed to configure systemd device allow for cgroup %s: %w", cgroupPath, err)
	}

	if cgroupMode == cgroups.Legacy {
		log.Debugf("Configuring PID %d cgroupv1 device allow, cgroup path %s", pid, cgroupPath)
		err = configureCgroupV1DeviceAllow(hostRoot, cgroupPath, nvidiaDeviceMajor)
	} else {
		log.Debugf("Configuring PID %d cgroupv2 device programs, cgroup path %s", pid, cgroupPath)
		err = detachAllDeviceCgroupPrograms(hostRoot, cgroupPath)
	}

	if err != nil {
		return fmt.Errorf("failed to configure cgroup device allow for cgroup path %s of PID %d: %w", cgroupPath, pid, err)
	}

	return nil
}

const (
	systemdDeviceAllowFile     = "50-DeviceAllow.conf"
	systemdTransientConfigPath = "run/systemd/transient"
	cgroupv1DeviceAllowFile    = "devices.allow"
	cgroupv1DeviceControlDir   = "sys/fs/cgroup/devices"
	nvidiaSystemdDeviceAllow   = "DeviceAllow=char-nvidia rwm\nDeviceAllow=char-195 rwm\n" // Allow access to the NVIDIA character devices
	nvidiaDeviceMajor          = 195
	cgroupFsPath               = "/sys/fs/cgroup"
)

// getAbsoluteCgroupForProcess gets the absolute cgroup path for a process independently of whether
// we are inside a container or not, or of thecgroup version being used.
// rootfs is the root filesystem path (usually /, but can be d ifferent to allow unit testing)
// hostRoot is the path to the host root filesystem, relative to rootfs
// currentProcessPid is the PID of the process currently running (os.Getpid(), but can be different for unit testing)
// targetProcessPid is the PID of the process whose cgroup we want to get
func getAbsoluteCgroupForProcess(rootfs, hostRoot string, currentProcessPid, targetProcessPid uint32, cgroupMode cgroups.CGMode) (string, error) {
	// Get the cgroup for the target process
	procCgroups, err := utils.GetProcControlGroups(targetProcessPid, targetProcessPid)
	if err != nil {
		return "", fmt.Errorf("failed to get cgroups for pid %d: %w", targetProcessPid, err)
	}

	if len(procCgroups) == 0 {
		return "", fmt.Errorf("no cgroups found for pid %d", targetProcessPid)
	}

	// Each cgroup is for a different subsystem in cgroupv1, we only want the cgroup ID
	// and we can extract that from any cgroup
	// In cgroupv2 we only have one cgroup, so this code also works.
	cgroupPath := string(procCgroups[0].Path)

	// If we're running in the host (no path to the host root filesystem), we can
	// just return the cgroup path as we see it, we cannot do anything else.
	// Also, cgroupv1 returns the cgroup name correctly here, so we can return it
	// directly too
	if rootfs == "" || cgroupMode != cgroups.Unified {
		return cgroupPath, nil
	}

	// Now we need to deal with possibly different cgroup namespaces, which can
	// happen in containerized environments. The cgroups we see from the
	// system-probe container are different from the cgroups we see from the
	// host. The cgroup extracted in the code above from /proc/pid/cgroup will
	// be valid only for the container cgroup namespace, but we want the cgroup
	// name in the host cgroup namespace.
	//
	// There are several possibilities to achieve this. The first, and easiest
	// to implement, would be to enter the root cgroup namespace and look at the
	// cgroup then. Unfortunately, this is not always possible as GKE seems to
	// deny containers the access to the root cgroup namespace even with
	// CAP_SYS_ADMIN capabilities.
	//
	// The other option is to match the container cgroup to a cgroup in the host
	// namespace using the cgroup directory from the host, the one that can be
	// seen in rootfs. Despite cgroups not having canonical IDs/names in their
	// directories, the inodes are unique and constant among namespaces.
	//
	// However, this only works as long the cgroup name returned in
	// /proc/pid/cgroup is absolute (i.e, /something). If it's relative (i.e,
	// /../something, which can happen if the target process is in a cgroup that
	// is a sibling of the current process' cgroup), then we need to resolve
	// first our own cgroup path and resolve the relative path based on that.
	pathParts := strings.Split(cgroupPath, "/")
	if len(pathParts) > 1 && pathParts[1] == ".." { // first part is an empty string as the cgroup path starts by /
		// Sanity check that we're not recursively getting our own cgroup, avoiding infinite recursion
		if targetProcessPid == currentProcessPid {
			return "", fmt.Errorf("impossible situation: got a relative path for our own cgroup %s for pid %d", cgroupPath, targetProcessPid)
		}

		currentCgroup, err := getAbsoluteCgroupForProcess(rootfs, hostRoot, currentProcessPid, currentProcessPid, cgroupMode)
		if err != nil {
			return "", fmt.Errorf("failed to get current process (pid=%d) cgroup: %w", currentProcessPid, err)
		}

		return filepath.Join(currentCgroup, cgroupPath), nil
	}

	// At this point we have an absolute cgroup path (possibly /, but can't know for sure).
	// Get the inode to the cgroup directory in our cgroup namespace.
	containerCgroupPath := filepath.Join(rootfs, cgroupFsPath, cgroupPath)
	var stat syscall.Stat_t
	if err = syscall.Stat(containerCgroupPath, &stat); err != nil {
		return "", fmt.Errorf("failed to stat container cgroup %s: %w", containerCgroupPath, err)
	}

	containerCgroupInode := stat.Ino

	// Now, walk through the host cgroup tree, looking for a cgroup with the same inode
	// as the container cgroup.
	// TODO: We could use the pkg/util/cgroup package but it doesn't detect correctly the host
	// cgroup mountpoint, once that's fixed we can use it instead of walking the cgroup tree.
	var hostCgroupPath string
	rootSysFsCgroup := filepath.Join(rootfs, hostRoot, cgroupFsPath)
	err = filepath.WalkDir(rootSysFsCgroup, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk path %s: %w", path, err)
		}
		if !d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil // Ignore the error, as it might be a symlink
		}

		// Get pre-computed stat info, otherwise stat it ourselves
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat == nil {
			var newStat syscall.Stat_t
			if err = syscall.Stat(path, &newStat); err != nil {
				return nil // Ignore this one
			}
			stat = &newStat
		}

		if stat.Ino == containerCgroupInode {
			hostCgroupPath = path
			return filepath.SkipDir
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to walk cgroup tree: %w", err)
	}

	if hostCgroupPath == "" {
		return "", fmt.Errorf("no host cgroup found for container cgroup %s", cgroupPath)
	}

	// The path returned by WalkDir is absolute, make it relative to the rootfs
	// to get the proper cgroup path
	hostCgroupPath, err = filepath.Rel(rootSysFsCgroup, hostCgroupPath)
	if err != nil {
		return "", fmt.Errorf("cannot obtain relative path from %s to %s: %w", rootSysFsCgroup, hostCgroupPath, err)
	}

	// Add leading slash, as that's removed by filepath.Rel and cgroup names
	// should always have that
	return "/" + hostCgroupPath, nil
}

// insertDeviceAllowLine adds the DeviceAllow line to the lines of a
// SystemD configuration file, in the specified section. It will add it at the end
// of all other DeviceAllow lines in the section so that it is not overridden.
func insertDeviceAllowLine(lines []string, sectionHeader, newLine string) ([]string, error) {
	candidateLineIndex := -1
	foundSectionHeader := false
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == sectionHeader {
			foundSectionHeader = true
			candidateLineIndex = i + 1
		} else if foundSectionHeader && strings.HasPrefix(line, "DeviceAllow=") {
			candidateLineIndex = i + 1
		} else if foundSectionHeader && strings.HasPrefix(line, "[") {
			// Another section header, stop searching
			break
		}
	}

	if candidateLineIndex == -1 {
		return nil, fmt.Errorf("failed to find section header %s", sectionHeader)
	}

	// Insert the new line in the detected position
	newLines := make([]string, len(lines)+1)
	copy(newLines, lines[:candidateLineIndex])
	newLines[candidateLineIndex] = newLine
	copy(newLines[candidateLineIndex+1:], lines[candidateLineIndex:])

	return newLines, nil
}

func configureSystemDAllow(rootfs, containerID string) error {
	// The SystemD device configuration might be either in a 50-DeviceAllow.conf
	// file in a service configuration directory, or in a service file directly.
	// Default to the 50-DeviceAllow.conf file and fall back to the service file
	// if it doesn't exist.
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

	configFile, err := os.OpenFile(configFilePath, os.O_RDWR, 0) // Permissions are not important here, we're not creating the file if it doesn't exist
	if err != nil {
		return fmt.Errorf("failed to open config file %s: %w", configFilePath, err)
	}
	defer configFile.Close()

	// Read the entire file into memory
	content, err := io.ReadAll(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configFilePath, err)
	}

	lines := strings.Split(string(content), "\n")

	// Insert the nvidiaSystemdDeviceAllow line after [Scope]
	newLines, err := insertDeviceAllowLine(lines, "[Scope]", nvidiaSystemdDeviceAllow)
	if err != nil {
		return fmt.Errorf("failed to insert device allow line in %s: %w", configFilePath, err)
	}

	// Write the modified content back to the file
	newContent := strings.Join(newLines, "\n")
	if err := configFile.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate config file %s: %w", configFilePath, err)
	}
	if _, err := configFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to start of config file %s: %w", configFilePath, err)
	}
	if _, err := configFile.WriteString(newContent); err != nil {
		return fmt.Errorf("failed to write modified config to %s: %w", configFilePath, err)
	}

	return nil
}

func configureCgroupV1DeviceAllow(hostRoot, cgroupPath string, majorNumber int) error {
	deviceAllowPath, err := buildSafePath(hostRoot, cgroupv1DeviceControlDir, cgroupPath, cgroupv1DeviceAllowFile)
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
// cgroupName is the name of the cgroup, e.g. "/kubepods.slice"
// hostRoot is the rootfs where /sys/fs/cgroup is mounted
func detachAllDeviceCgroupPrograms(hostRoot, cgroupName string) error {
	cgroupHostPath, err := buildSafePath(hostRoot, "sys/fs/cgroup", cgroupName)
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
		// Detach the program
		if err := detachDeviceCgroupProgram(prog, int(cgroup.Fd())); err != nil {
			detachErrs = errors.Join(detachErrs, fmt.Errorf("failed to detach program %d from cgroup %s: %w", prog.ID, cgroupName, err))
			continue
		}

		log.Debugf("successfully detached device program %d from cgroup %s", prog.ID, cgroupName)
	}

	return detachErrs
}

func detachDeviceCgroupProgram(prog link.AttachedProgram, cgroupFd int) error {
	// Load the program by ID
	program, err := ebpf.NewProgramFromID(prog.ID)
	if err != nil {
		return err
	}
	defer program.Close()

	// Detach the program
	err = link.RawDetachProgram(link.RawDetachProgramOptions{
		Target:  cgroupFd,
		Program: program,
		Attach:  ebpf.AttachCGroupDevice,
	})
	if err != nil {
		return err
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
