// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package procfs

import (
	"fmt"
	"os"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func (c *Collector) getPath(request sbom.ScanRequest) (string, error) {
	pids, err := process.Pids()
	if err != nil {
		return "", err
	}

	selfPid := uint32(os.Getpid())
	selfCid, err := utils.GetProcContainerID(selfPid, selfPid)
	if err != nil {
		return "", err
	}

	for _, pid := range pids {
		cid, err := utils.GetProcContainerID(uint32(pid), uint32(pid))
		if err != nil || cid == "" {
			continue
		}

		if cid == containerutils.ContainerID(request.ID()) && cid != selfCid {
			fmt.Printf("%d CID: %s => %v\n", pid, cid, err)

			return utils.ProcRootPath(uint32(pid)), nil
		}
	}

	return "", ErrNotFound
}
