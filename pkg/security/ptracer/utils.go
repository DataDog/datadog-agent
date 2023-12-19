// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"bufio"
	"bytes"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/common/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

// Funcs mainly copied from github.com/DataDog/datadog-agent/pkg/security/utils/cgroup.go
// in order to reduce the binary size of cws-instrumentation

type controlGroup struct {
	// id unique hierarchy ID
	id int

	// controllers are the list of cgroup controllers bound to the hierarchy
	controllers []string

	// path is the pathname of the control group to which the process
	// belongs. It is relative to the mountpoint of the hierarchy.
	path string
}

func getProcControlGroupsFromFile(path string) ([]controlGroup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cgroups []controlGroup
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		t := scanner.Text()
		parts := strings.Split(t, ":")
		var ID int
		ID, err = strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		c := controlGroup{
			id:          ID,
			controllers: strings.Split(parts[1], ","),
			path:        parts[2],
		}
		cgroups = append(cgroups, c)
	}
	return cgroups, nil

}

func getCurrentProcContainerID() (string, error) {
	cgroups, err := getProcControlGroupsFromFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}

	for _, cgroup := range cgroups {
		cid := containerutils.FindContainerID(cgroup.path)
		if cid != "" {
			return cid, nil
		}
	}
	return "", nil
}

func retrieveContainerIDFromProc(ctx *ebpfless.ContainerContext) error {
	cgroup, err := getCurrentProcContainerID()
	if err != nil {
		return err
	}
	ctx.ID = cgroup
	return nil
}

func getNSID() uint64 {
	var stat syscall.Stat_t
	if err := syscall.Lstat("/proc/self/ns/pid", &stat); err != nil {
		return rand.Uint64()
	}
	return stat.Ino
}
