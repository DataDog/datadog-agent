// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package procfs

import (
	"bytes"
	"os"
	"strconv"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func cgroupContains(pid uint32, str string) (bool, error) {
	cgroupPath := kernel.HostProc(strconv.FormatUint(uint64(pid), 10), "cgroup")

	data, err := os.ReadFile(cgroupPath)
	if err != nil {
		return false, err
	}

	return bytes.Contains(data, []byte(str)), nil
}

func procRootPath(pid uint32) string {
	return kernel.HostProc(strconv.FormatUint(uint64(pid), 10), "root")
}

// IsAgentContainer returns whether the container ID is the agent one
func IsAgentContainer(ctrID string) (bool, error) {
	pid := os.Getpid()
	return cgroupContains(uint32(pid), ctrID)
}

func (c *Collector) getPath(request sbom.ScanRequest) (string, error) {
	pids, err := process.Pids()
	if err != nil {
		return "", err
	}

	for _, pid := range pids {
		if ok, _ := cgroupContains(uint32(pid), request.ID()); ok {
			return procRootPath(uint32(pid)), nil
		}
	}

	return "", ErrNotFound
}
