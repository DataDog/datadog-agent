// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package procfs

import (
	"strconv"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func procRootPath(pid uint32) string {
	return kernel.HostProc(strconv.FormatUint(uint64(pid), 10), "root")
}

func getPath(request sbom.ScanRequest) (string, error) {
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
