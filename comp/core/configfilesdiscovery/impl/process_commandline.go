// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker || (cri && containerd)

package configfilesdiscoveryimpl

import (
	"slices"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// readContainerProcessCommandlines returns workloadmeta command-line snapshots
// for processes associated with the target container. Workloadmeta owns the
// process lifecycle and metadata; this fallback deliberately avoids direct
// procfs access because the core Agent may not have permission to inspect the
// target process.
func readContainerProcessCommandlines(store workloadmeta.Component, containerID string) []TargetCommandline {
	if store == nil {
		return nil
	}

	var commandlines []TargetCommandline
	processes := store.ListProcessesWithFilter(func(process *workloadmeta.Process) bool {
		return process != nil && process.ContainerID == containerID
	})
	for _, workloadmetaProcess := range processes {
		if workloadmetaProcess.Pid <= 0 || len(workloadmetaProcess.Cmdline) == 0 {
			continue
		}

		commandlines = append(commandlines, TargetCommandline{
			Args:       slices.Clone(workloadmetaProcess.Cmdline),
			WorkingDir: workloadmetaProcess.Cwd,
		})
	}
	return commandlines
}
