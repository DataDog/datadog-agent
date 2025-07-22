// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package utils holds utils related files
package utils

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/moby/sys/mountinfo"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// ContainerIDLen is the length of a container ID is the length of the hex representation of a sha256 hash
const ContainerIDLen = sha256.Size * 2

// ControlGroup describes the cgroup membership of a process
type ControlGroup struct {
	// ID unique hierarchy ID
	ID int

	// Controllers are the list of cgroup controllers bound to the hierarchy
	Controllers []string

	// Path is the pathname of the control group to which the process
	// belongs. It is relative to the mountpoint of the hierarchy.
	Path string
}

// GetContainerContext returns both the container ID and its flags
func (cg ControlGroup) GetContainerContext() (containerutils.ContainerID, containerutils.CGroupFlags) {
	return containerutils.FindContainerID(containerutils.CGroupID(cg.Path))
}

func parseCgroupLine(line string) (string, string, string, error) {
	id, rest, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", "", fmt.Errorf("invalid cgroup line: %s", line)
	}

	ctrl, path, ok := strings.Cut(rest, ":")
	if !ok {
		return "", "", "", fmt.Errorf("invalid cgroup line: %s", line)
	}

	if rest == "/" {
		return "", "", "", fmt.Errorf("invalid cgroup line: %s", line)
	}

	return id, ctrl, path, nil
}

func parseProcControlGroupsData(data []byte, fnc func(string, string, string) bool) error {
	data = bytes.TrimSpace(data)

	for len(data) != 0 {
		eol := bytes.IndexByte(data, '\n')
		if eol < 0 {
			eol = len(data)
		}
		line := data[:eol]

		id, ctrl, path, err := parseCgroupLine(string(line))
		if err != nil {
			return err
		}

		if fnc(id, ctrl, path) {
			return nil
		}

		nextStart := eol + 1
		if nextStart >= len(data) {
			break
		}
		data = data[nextStart:]
	}

	return nil
}

func parseProcControlGroups(tgid, pid uint32, fnc func(string, string, string) bool) error {
	data, err := os.ReadFile(CgroupTaskPath(tgid, pid))
	if err != nil {
		return err
	}
	return parseProcControlGroupsData(data, fnc)
}

func makeControlGroup(id, ctrl, path string) (ControlGroup, error) {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return ControlGroup{}, err
	}

	return ControlGroup{
		ID:          idInt,
		Controllers: strings.Split(ctrl, ","),
		Path:        path,
	}, nil
}

// GetProcControlGroups returns the cgroup membership of the specified task.
func GetProcControlGroups(tgid, pid uint32) ([]ControlGroup, error) {
	data, err := os.ReadFile(CgroupTaskPath(tgid, pid))
	if err != nil {
		return nil, err
	}
	var cgroups []ControlGroup
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		t := scanner.Text()
		id, ctrl, path, err := parseCgroupLine(t)
		if err != nil {
			return nil, err
		}

		cgroup, err := makeControlGroup(id, ctrl, path)
		if err != nil {
			return nil, err
		}

		cgroups = append(cgroups, cgroup)
	}
	return cgroups, nil
}

// GetProcContainerID returns the container ID which the process belongs to. Returns "" if the process does not belong
// to a container.
func GetProcContainerID(tgid, pid uint32) (containerutils.ContainerID, error) {
	id, _, err := GetProcContainerContext(tgid, pid)
	return id, err
}

// CGroupContext holds the cgroup context of a process
type CGroupContext struct {
	CGroupID          containerutils.CGroupID
	CGroupFlags       containerutils.CGroupFlags
	CGroupFileMountID uint32
	CGroupFileInode   uint64
}

// GetProcContainerContext returns the container ID which the process belongs to along with its manager. Returns "" if the process does not belong
// to a container.
func GetProcContainerContext(tgid, pid uint32) (containerutils.ContainerID, CGroupContext, error) {
	var (
		containerID   containerutils.ContainerID
		runtime       containerutils.CGroupFlags
		cgroupContext CGroupContext
	)

	if err := parseProcControlGroups(tgid, pid, func(id, ctrl, path string) bool {
		if path == "/" {
			return false
		} else if ctrl != "" && !strings.HasPrefix(ctrl, "name=") {
			// On cgroup v1 we choose to take the "name" ctrl entry (ID 1), as the ID 0 could be empty
			// On cgroup v2, it's only a single line with ID 0 and no ctrl
			// (Cf unit tests for examples)
			return false
		}
		cgroup, err := makeControlGroup(id, ctrl, path)
		if err != nil {
			return false
		}

		containerID, runtime = cgroup.GetContainerContext()
		cgroupContext.CGroupID = containerutils.CGroupID(cgroup.Path)
		cgroupContext.CGroupFlags = runtime

		return true
	}); err != nil {
		return "", CGroupContext{}, err
	}

	var fileStats unix.Statx_t
	taskPath := CgroupTaskPath(pid, pid)
	if err := unix.Statx(unix.AT_FDCWD, taskPath, 0, unix.STATX_INO|unix.STATX_MNT_ID, &fileStats); err == nil {
		cgroupContext.CGroupFileMountID = uint32(fileStats.Mnt_id)
		cgroupContext.CGroupFileInode = fileStats.Ino
	}

	return containerID, cgroupContext, nil
}

var defaultCGroupMountpoints = []string{
	"/sys/fs/cgroup",
	"/sys/fs/cgroup/unified",
}

// ErrNoCGroupMountpoint is returned when no cgroup mount point is found
var ErrNoCGroupMountpoint = errors.New("no cgroup mount point found")

// CGroupFS is a helper type used to find the cgroup context of a process
type CGroupFS struct {
	cGroupMountPoints []string
}

// NewCGroupFS creates a new CGroupFS instance
func NewCGroupFS(cgroupMountPoints ...string) *CGroupFS {
	cfs := &CGroupFS{}

	var cgroupMnts []string
	if len(cgroupMountPoints) == 0 {
		cgroupMnts = defaultCGroupMountpoints
	} else {
		cgroupMnts = cgroupMountPoints
	}

	for _, mountpoint := range cgroupMnts {
		hostMountpoint := filepath.Join(kernel.SysFSRoot(), strings.TrimPrefix(mountpoint, "/sys/"))
		if mounted, _ := mountinfo.Mounted(hostMountpoint); mounted {
			cfs.cGroupMountPoints = append(cfs.cGroupMountPoints, hostMountpoint)
		}
	}

	return cfs
}

// FindCGroupContext returns the container ID, cgroup context and sysfs cgroup path the process belongs to.
// Returns "" as container ID and sysfs cgroup path, and an empty CGroupContext if the process does not belong to a container.
func (cfs *CGroupFS) FindCGroupContext(tgid, pid uint32) (containerutils.ContainerID, CGroupContext, string, error) {
	if len(cfs.cGroupMountPoints) == 0 {
		return "", CGroupContext{}, "", ErrNoCGroupMountpoint
	}

	var (
		containerID     containerutils.ContainerID
		cgroupContext   CGroupContext
		sysFScGroupPath string
	)

	err := parseProcControlGroups(tgid, pid, func(_, ctrl, path string) bool {
		if path == "/" {
			return false
		} else if ctrl != "" && !strings.HasPrefix(ctrl, "name=") {
			// On cgroup v1 we choose to take the "name" ctrl entry (ID 1), as the ID 0 could be empty
			// On cgroup v2, it's only a single line with ID 0 and no ctrl
			// (Cf unit tests for examples)
			return false
		}

		ctrlDirectory := strings.TrimPrefix(ctrl, "name=")
		for _, mountpoint := range cfs.cGroupMountPoints {
			cgroupPath := filepath.Join(mountpoint, ctrlDirectory, path)
			if exists, err := checkPidExists(cgroupPath, pid); err == nil && exists {
				cgroupID := containerutils.CGroupID(path)
				ctrID, flags := containerutils.FindContainerID(cgroupID)
				cgroupContext.CGroupID = cgroupID
				cgroupContext.CGroupFlags = containerutils.CGroupFlags(flags)
				containerID = ctrID
				sysFScGroupPath = cgroupPath

				var fileStatx unix.Statx_t
				var fileStats unix.Stat_t
				if err := unix.Statx(unix.AT_FDCWD, sysFScGroupPath, 0, unix.STATX_INO|unix.STATX_MNT_ID, &fileStatx); err == nil {
					cgroupContext.CGroupFileMountID = uint32(fileStatx.Mnt_id)
					cgroupContext.CGroupFileInode = fileStatx.Ino
				} else if err := unix.Stat(sysFScGroupPath, &fileStats); err == nil {
					cgroupContext.CGroupFileInode = fileStats.Ino
				}
				return true
			}
		}
		return false
	})
	if err != nil {
		return "", CGroupContext{}, "", err
	}

	return containerID, cgroupContext, sysFScGroupPath, nil
}

func checkPidExists(sysFScGroupPath string, expectedPid uint32) (bool, error) {
	data, err := os.ReadFile(filepath.Join(sysFScGroupPath, "cgroup.procs"))
	if err != nil {
		// the cgroup is in threaded mode, and in that case, reading cgroup.procs returns ENOTSUP.
		// see https://github.com/opencontainers/runc/issues/3821
		if errors.Is(err, unix.ENOTSUP) {
			if data, err = os.ReadFile(filepath.Join(sysFScGroupPath, "cgroup.threads")); err != nil {
				return false, err
			}
		} else {
			return false, err
		}
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if pid, err := strconv.Atoi(strings.TrimSpace(scanner.Text())); err == nil && uint32(pid) == expectedPid {
			return true, nil
		}
	}
	return false, nil
}

// GetCgroup2MountPoint checks if cgroup v2 is available and returns its mount point
func GetCgroup2MountPoint() (string, error) {
	file, err := os.Open(kernel.HostProc("/1/mountinfo"))
	if err != nil {
		return "", fmt.Errorf("couldn't resolve cgroup2 mount point: failed to open /proc/self/mountinfo: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		// The cgroup2 mount entry will have "cgroup2" as the filesystem type
		if len(fields) >= 10 && fields[len(fields)-3] == "cgroup2" {
			// The 5th field is the mount point
			return filepath.Join(kernel.SysFSRoot(), strings.TrimPrefix(fields[4], "/sys")), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("couldn't resolve cgroup2 mount point: error reading mountinfo: %w", err)
	}

	return "", nil
}
