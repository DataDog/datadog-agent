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
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/common/containerutils"
)

// ContainerID is the type holding the container ID
type ContainerID string

// ContainerFlags is the type holding the container flags
type ContainerFlags uint64

// Bytes returns the container ID as a byte array
func (c ContainerID) Bytes() []byte {
	buff := make([]byte, ContainerIDLen)
	if len(c) == ContainerIDLen {
		copy(buff[:], c)
	}
	return buff
}

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

// GetContainerContainer returns both the container ID and its manager
func (cg ControlGroup) GetContainerContext() (ContainerID, ContainerFlags) {
	id, flags := containerutils.FindContainerID(cg.Path)
	return ContainerID(id), ContainerFlags(flags)
}

// GetContainerID returns the container id extracted from the path of the control group
func (cg ControlGroup) GetContainerID() ContainerID {
	id, _ := containerutils.FindContainerID(cg.Path)
	return ContainerID(id)
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
func GetProcContainerID(tgid, pid uint32) (ContainerID, error) {
	id, _, err := GetProcContainerContext(tgid, pid)
	return id, err
}

// GetProcContainerContext returns the container ID which the process belongs to along with its manager. Returns "" if the process does not belong
// to a container.
func GetProcContainerContext(tgid, pid uint32) (ContainerID, ContainerFlags, error) {
	cgroups, err := GetProcControlGroups(tgid, pid)
	if err != nil {
		return "", 0, err
	}

	for _, cgroup := range cgroups {
		if containerID, runtime := cgroup.GetContainerContext(); containerID != "" {
			return containerID, runtime, nil
		}
	}
	return "", 0, nil
}
