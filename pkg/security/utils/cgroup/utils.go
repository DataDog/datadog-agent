// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package cgroup parses /proc cgroup membership data without cgroup filesystem access.
package cgroup

import (
	"bufio"
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

// ControlGroup describes the cgroup membership of a process (paths from /proc).
type ControlGroup struct {
	// ID unique hierarchy ID
	ID int

	// Controllers are the list of cgroup controllers bound to the hierarchy
	Controllers []string

	// Path is the pathname of the control group to which the process
	// belongs. It is relative to the mountpoint of the hierarchy.
	Path string
}

// GetContainerContext returns the container ID derived from the cgroup path.
func (cg ControlGroup) GetContainerContext() containerutils.ContainerID {
	return containerutils.FindContainerID(containerutils.CGroupID(cg.Path))
}

// ParseCgroupLine parses a single line from /proc/[pid]/cgroup.
func ParseCgroupLine(line string) (id, ctrl, path string, err error) {
	id, rest, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", "", fmt.Errorf("invalid cgroup line: %s", line)
	}

	ctrl, path, ok = strings.Cut(rest, ":")
	if !ok {
		return "", "", "", fmt.Errorf("invalid cgroup line: %s", line)
	}

	return id, ctrl, path, nil
}

// MakeControlGroup builds a ControlGroup from fields returned by ParseCgroupLine.
func MakeControlGroup(id, ctrl, path string) (ControlGroup, error) {
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

// ParseProcCgroupDataStrict parses the full /proc/[pid]/cgroup file; any non-empty invalid line is an error.
func ParseProcCgroupDataStrict(data []byte) ([]ControlGroup, error) {
	var cgroups []ControlGroup
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		t := scanner.Text()
		id, ctrl, path, err := ParseCgroupLine(t)
		if err != nil {
			return nil, err
		}

		cgroup, err := MakeControlGroup(id, ctrl, path)
		if err != nil {
			return nil, err
		}

		cgroups = append(cgroups, cgroup)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cgroups, nil
}

// ParseProcCgroupDataLenient parses /proc/[pid]/cgroup: trims lines, skips empty lines,
// and skips lines that cannot be parsed or have a non-integer hierarchy id.
func ParseProcCgroupDataLenient(data []byte) []ControlGroup {
	var cgroups []ControlGroup
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		t := strings.TrimSpace(scanner.Text())
		if t == "" {
			continue
		}
		id, ctrl, path, err := ParseCgroupLine(t)
		if err != nil {
			continue
		}
		cg, err := MakeControlGroup(id, ctrl, path)
		if err != nil {
			continue
		}
		cgroups = append(cgroups, cg)
	}
	return cgroups
}

// FindContainerIDFromEntries extracts a container ID from parsed /proc cgroup lines.
func FindContainerIDFromEntries(cgroups []ControlGroup) containerutils.ContainerID {
	for _, cgroup := range cgroups {
		str := cgroup.Path
		if strings.Contains(cgroup.Path, "kubepods") {
			els := strings.Split(str, "/")
			if len(els) > 0 {
				str = els[len(els)-1]
			}
		}
		if cid := containerutils.FindContainerID(containerutils.CGroupID(str)); cid != "" {
			return cid
		} else if cid = containerutils.FindContainerID(containerutils.CGroupID(cgroup.Path)); cid != "" {
			return cid
		}
	}
	return ""
}

// CGroupIDFromEntries picks a cgroup path string suitable as CGroupID from parsed /proc cgroup lines.
func CGroupIDFromEntries(cgroups []ControlGroup) containerutils.CGroupID {
	var fallback containerutils.CGroupID
	for _, cgroup := range cgroups {
		if cgroup.Path == "" || cgroup.Path == "/" {
			continue
		}

		cgroupID := containerutils.CGroupID(cgroup.Path)
		if slices.Contains(cgroup.Controllers, "") || slices.Contains(cgroup.Controllers, "name=systemd") {
			return cgroupID
		}

		if fallback == "" {
			fallback = cgroupID
		}
	}

	return fallback
}

// ContainerContextFromProcCgroupData returns container ID and cgroup path id from a single parse of the proc cgroup file.
func ContainerContextFromProcCgroupData(data []byte) (containerutils.ContainerID, containerutils.CGroupID) {
	entries := ParseProcCgroupDataLenient(data)
	return FindContainerIDFromEntries(entries), CGroupIDFromEntries(entries)
}
