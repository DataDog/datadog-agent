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
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"golang.org/x/sys/unix"
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
	id, flags := containerutils.FindContainerID(containerutils.CGroupID(cg.Path))
	return containerutils.ContainerID(id), containerutils.CGroupFlags(flags)
}

// GetContainerID returns the container id extracted from the path of the control group
func (cg ControlGroup) GetContainerID() containerutils.ContainerID {
	id, _ := containerutils.FindContainerID(containerutils.CGroupID(cg.Path))
	return containerutils.ContainerID(id)
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

	var lastLine []byte

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

		if bytes.ContainsRune(line, ':') {
			lastLine = line
		}

		nextStart := eol + 1
		if nextStart >= len(data) {
			break
		}
		data = data[nextStart:]
	}

	id, ctrl, path, err := parseCgroupLine(string(lastLine))
	if err != nil {
		return err
	}

	if fnc(id, ctrl, path) {
		return nil
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

// GetProcContainerContext returns the container ID which the process belongs to along with its manager. Returns "" if the process does not belong
// to a container.
func GetProcContainerContext(tgid, pid uint32) (containerutils.ContainerID, model.CGroupContext, error) {
	var (
		containerID   containerutils.ContainerID
		runtime       containerutils.CGroupFlags
		cgroupContext model.CGroupContext
	)

	if err := parseProcControlGroups(tgid, pid, func(id, ctrl, path string) bool {
		if path == "/" {
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
		return "", model.CGroupContext{}, err
	}

	var fileStats unix.Statx_t
	taskPath := CgroupTaskPath(pid, pid)
	if err := unix.Statx(unix.AT_FDCWD, taskPath, 0, unix.STATX_INO|unix.STATX_MNT_ID, &fileStats); err == nil {
		cgroupContext.CGroupFile.MountID = uint32(fileStats.Mnt_id)
		cgroupContext.CGroupFile.Inode = fileStats.Ino
	}

	return containerID, cgroupContext, nil
}
