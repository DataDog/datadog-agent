// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procfs

import (
	"bytes"
	"os"
	"strconv"

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

// IsAgentContainer returns whether the container ID is the agent one
func IsAgentContainer(ctrID string) (bool, error) {
	pid := os.Getpid()
	return cgroupContains(uint32(pid), ctrID)
}
