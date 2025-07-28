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
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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

func parseProcControlGroupsData(data []byte, validateCgroupEntry func(string, string, string) (bool, error)) error {
	data = bytes.TrimSpace(data)

	var (
		id      string
		ctrl    string
		path    string
		isValid bool
		err     error
	)
	for len(data) != 0 {
		eol := bytes.IndexByte(data, '\n')
		if eol < 0 {
			eol = len(data)
		}
		line := data[:eol]

		id, ctrl, path, err = parseCgroupLine(string(line))
		if err != nil {
			return err
		}

		if isValid, err = validateCgroupEntry(id, ctrl, path); isValid {
			return nil
		}

		nextStart := eol + 1
		if nextStart >= len(data) {
			break
		}
		data = data[nextStart:]
	}

	// return the lastest error
	return err
}

func parseProcControlGroups(tgid, pid uint32, validateCgroupEntry func(string, string, string) (bool, error)) error {
	data, err := os.ReadFile(CgroupTaskPath(tgid, pid))
	if err != nil {
		return err
	}
	return parseProcControlGroupsData(data, validateCgroupEntry)
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

// CGroupContext holds the cgroup context of a process
type CGroupContext struct {
	CGroupID          containerutils.CGroupID
	CGroupFlags       containerutils.CGroupFlags
	CGroupFileMountID uint32
	CGroupFileInode   uint64
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
	rootCGroupPath    string
}

var defaultCGroupFS *CGroupFS
var once sync.Once

// DefaultCGroupFS returns a singleton instance of CGroupFS
func DefaultCGroupFS() *CGroupFS {
	once.Do(func() {
		defaultCGroupFS = newCGroupFS()
	})
	return defaultCGroupFS
}

// newCGroupFS creates a new CGroupFS instance
func newCGroupFS() *CGroupFS {
	cfs := &CGroupFS{}

	for _, mountpoint := range defaultCGroupMountpoints {
		hostMountpoint := filepath.Join(kernel.SysFSRoot(), strings.TrimPrefix(mountpoint, "/sys/"))
		if mounted, _ := mountinfo.Mounted(hostMountpoint); mounted {
			cfs.cGroupMountPoints = append(cfs.cGroupMountPoints, hostMountpoint)
		}
	}

	// detect the current cgroup path in the pid namespace
	cfs.detectCurrentCgroupPath(Getpid(), uint32(os.Getpid()))

	return cfs
}

// FindCGroupContext returns the container ID, cgroup context and sysfs cgroup path the process belongs to.
// Returns "" as container ID and sysfs cgroup path, and an empty CGroupContext if the process does not belong to a container.
func (cfs *CGroupFS) FindCGroupContext(tgid, pid uint32) (containerutils.ContainerID, CGroupContext, string, error) {
	if len(cfs.cGroupMountPoints) == 0 {
		return "", CGroupContext{}, "", ErrNoCGroupMountpoint
	}

	var (
		containerID   containerutils.ContainerID
		cgroupContext CGroupContext
		cgroupPath    string
	)

	err := parseProcControlGroups(tgid, pid, func(_, ctrl, path string) (bool, error) {
		if path == "/" && cfs.rootCGroupPath == "" {
			return false, nil
		} else if ctrl != "" && !strings.HasPrefix(ctrl, "name=") {
			// On cgroup v1 we choose to take the "name" ctrl entry (ID 1), as the ID 0 could be empty
			// On cgroup v2, it's only a single line with ID 0 and no ctrl
			// (Cf unit tests for examples)
			return false, nil
		}

		var (
			exists bool
			err    error
		)

		ctrlDirectory := strings.TrimPrefix(ctrl, "name=")
		for _, mountpoint := range cfs.cGroupMountPoints {
			// in case of relative path use rootCgroupPath
			if strings.HasPrefix(path, "/..") || path == "/" {
				cgroupPath = filepath.Join(cfs.rootCGroupPath, path)
			} else {
				cgroupPath = filepath.Join(mountpoint, ctrlDirectory, path)
			}

			if exists, err = checkPidExists(cgroupPath, pid); err == nil && exists {
				cgroupID := containerutils.CGroupID(cgroupPath)
				ctrID, flags := containerutils.FindContainerID(cgroupID)
				cgroupContext.CGroupID = cgroupID
				cgroupContext.CGroupFlags = containerutils.CGroupFlags(flags)
				containerID = ctrID

				var (
					fileStatx unix.Statx_t
					fileStats unix.Stat_t
				)

				if err = unix.Statx(unix.AT_FDCWD, cgroupPath, 0, unix.STATX_INO|unix.STATX_MNT_ID, &fileStatx); err == nil {
					cgroupContext.CGroupFileMountID = uint32(fileStatx.Mnt_id)
					cgroupContext.CGroupFileInode = fileStatx.Ino
				} else if err = unix.Stat(cgroupPath, &fileStats); err == nil {
					cgroupContext.CGroupFileInode = fileStats.Ino
				}

				return true, err
			}
		}

		// return the lastest error
		return false, err
	})
	if err != nil {
		return "", CGroupContext{}, "", err
	}

	return containerID, cgroupContext, cgroupPath, nil
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

func isInCGroupNamespace(pid uint32) (bool, error) {
	cgroups, err := GetProcControlGroups(pid, pid)
	if err != nil {
		return false, err
	}

	if len(cgroups) > 0 {
		return cgroups[0].Path == "/", nil
	}

	return false, nil
}

// DetectCurrentCgroupPath returns the cgroup path of the current process
func (cfs *CGroupFS) detectCurrentCgroupPath(currentPid, currentNSPid uint32) {
	// check if in a namespace
	if inNamespace, err := isInCGroupNamespace(currentPid); err != nil || !inNamespace {
		return
	}

	grepPid := func(dir string) string {
		var cgroupPath string

		_ = filepath.WalkDir(dir, func(path string, _ fs.DirEntry, _ error) error {
			if filepath.Base(path) == "cgroup.procs" || filepath.Base(path) == "cgroup.threads" {
				data, err := os.ReadFile(path)
				if err == nil {
					scanner := bufio.NewScanner(bytes.NewReader(data))
					for scanner.Scan() {
						if pid, err := strconv.Atoi(strings.TrimSpace(scanner.Text())); err == nil && uint32(pid) == currentNSPid {
							cgroupPath = filepath.Dir(path)
							return fs.SkipAll
						}
					}
				}
			}
			return nil
		})
		return cgroupPath
	}

	var rootCGroupPath string
	for _, mountpoint := range cfs.cGroupMountPoints {
		if cgroupPath := grepPid(mountpoint); cgroupPath != "" {
			rootCGroupPath = cgroupPath
			break
		}
	}

	cfs.rootCGroupPath = rootCGroupPath
}

// GetRootCGroupPath returns the root cgroup path
func (cfs *CGroupFS) GetRootCGroupPath() string {
	return cfs.rootCGroupPath
}
