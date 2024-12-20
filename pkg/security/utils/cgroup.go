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

// GetLastProcControlGroups returns the first cgroup membership of the specified task.
func GetLastProcControlGroups(tgid, pid uint32) (ControlGroup, error) {
	data, err := os.ReadFile(CgroupTaskPath(tgid, pid))
	if err != nil {
		return ControlGroup{}, err
	}

	data = bytes.TrimSpace(data)

	index := bytes.LastIndexByte(data, '\n')
	if index < 0 {
		index = 0
	} else {
		index++ // to skip the \n
	}
	if index >= len(data) {
		return ControlGroup{}, fmt.Errorf("invalid cgroup data: %s", data)
	}

	lastLine := string(data[index:])

	idstr, rest, ok := strings.Cut(lastLine, ":")
	if !ok {
		return ControlGroup{}, fmt.Errorf("invalid cgroup line: %s", lastLine)
	}

	id, err := strconv.Atoi(idstr)
	if err != nil {
		return ControlGroup{}, err
	}

	controllers, path, ok := strings.Cut(rest, ":")
	if !ok {
		return ControlGroup{}, fmt.Errorf("invalid cgroup line: %s", lastLine)
	}

	return ControlGroup{
		ID:          id,
		Controllers: strings.Split(controllers, ","),
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
		parts := strings.Split(t, ":")
		var ID int
		ID, err = strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		c := ControlGroup{
			ID:          ID,
			Controllers: strings.Split(parts[1], ","),
			Path:        parts[2],
		}
		cgroups = append(cgroups, c)
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
	cgroup, err := GetLastProcControlGroups(tgid, pid)
	if err != nil {
		return "", model.CGroupContext{}, err
	}

	containerID, runtime := cgroup.GetContainerContext()
	cgroupContext := model.CGroupContext{
		CGroupID:    containerutils.CGroupID(cgroup.Path),
		CGroupFlags: runtime,
	}

	var fileStats unix.Statx_t
	taskPath := CgroupTaskPath(pid, pid)
	if err := unix.Statx(unix.AT_FDCWD, taskPath, 0, unix.STATX_INO|unix.STATX_MNT_ID, &fileStats); err == nil {
		cgroupContext.CGroupFile.MountID = uint32(fileStats.Mnt_id)
		cgroupContext.CGroupFile.Inode = fileStats.Ino
	}

	return containerID, cgroupContext, nil
}
