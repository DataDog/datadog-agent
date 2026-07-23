// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"slices"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/shirou/gopsutil/v4/process"
)

// readContainerProcessCommandlines returns command lines for processes that
// workloadmeta associates with the target container after validating that each
// workloadmeta entry still describes the corresponding live process.
func readContainerProcessCommandlines(ctx context.Context, store workloadmeta.Component, containerID string, readProcessWorkingDir func(context.Context, *workloadmeta.Process) (string, bool)) []TargetCommandline {
	if store == nil {
		return nil
	}

	var commandlines []TargetCommandline
	processes := store.ListProcessesWithFilter(func(process *workloadmeta.Process) bool {
		return process != nil && process.ContainerID == containerID
	})
	for _, workloadmetaProcess := range processes {
		if workloadmetaProcess.Pid <= 0 || workloadmetaProcess.CreationTime.IsZero() || len(workloadmetaProcess.Cmdline) == 0 {
			continue
		}

		workingDir, ok := readProcessWorkingDir(ctx, workloadmetaProcess)
		if !ok {
			continue
		}
		commandlines = append(commandlines, TargetCommandline{
			Args:       slices.Clone(workloadmetaProcess.Cmdline),
			WorkingDir: workingDir,
		})
	}
	return commandlines
}

// matchesProcessCreationTime compares at workloadmeta's process timestamp
// precision. Some workloadmeta sources record process start times in seconds.
func matchesProcessCreationTime(workloadmetaTime time.Time, liveTime time.Time) bool {
	if workloadmetaTime.Equal(liveTime) {
		return true
	}
	return workloadmetaTime.Nanosecond() == 0 && workloadmetaTime.Unix() == liveTime.Unix()
}

func matchesLiveProcessIdentity(workloadmetaProcess *workloadmeta.Process, liveCommandline []string, liveExecutable string) bool {
	if workloadmetaProcess.Exe != "" {
		return liveExecutable != "" && workloadmetaProcess.Exe == liveExecutable
	}
	return len(liveCommandline) > 0 && slices.Equal(workloadmetaProcess.Cmdline, liveCommandline)
}

// readLiveProcessWorkingDir returns the working directory and true when the
// workloadmeta entry still identifies the same live, non-zombie process.
func readLiveProcessWorkingDir(ctx context.Context, workloadmetaProcess *workloadmeta.Process) (string, bool) {
	liveProcess, err := process.NewProcessWithContext(ctx, workloadmetaProcess.Pid)
	if err != nil {
		return "", false
	}

	creationTimeMillis, err := liveProcess.CreateTimeWithContext(ctx)
	if err != nil || !matchesProcessCreationTime(workloadmetaProcess.CreationTime, time.UnixMilli(creationTimeMillis).UTC()) {
		return "", false
	}
	statuses, err := liveProcess.StatusWithContext(ctx)
	if err != nil {
		return "", false
	}
	if slices.Contains(statuses, process.Zombie) {
		return "", false
	}

	commandline, _ := liveProcess.CmdlineSliceWithContext(ctx)
	executable, _ := liveProcess.ExeWithContext(ctx)
	if !matchesLiveProcessIdentity(workloadmetaProcess, commandline, executable) {
		return "", false
	}
	workingDir, err := liveProcess.CwdWithContext(ctx)
	if err != nil {
		workingDir = ""
	}

	running, err := liveProcess.IsRunningWithContext(ctx)
	if err != nil {
		return "", false
	}
	if !running {
		return "", false
	}

	return workingDir, true
}
