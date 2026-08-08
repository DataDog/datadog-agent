// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	// list of binaries that can't be killed
	binariesExcluded = []string{}
)

type killContext struct {
	pid  int
	path string
}

// cgroupKillTarget identifies a cgroup to kill in a single operation. Cgroups don't exist on
// Windows, so the ID is never set and the one-shot kill path is never taken.
type cgroupKillTarget struct {
	id containerutils.CGroupID
}

// ProcessKillerWindows defines the process kill windows implementation
type ProcessKillerWindows struct{}

// NewProcessKillerOS returns a ProcessKillerOS
// The second parameter (cgroupResolver) is ignored on Windows as container scope is not supported
func NewProcessKillerOS(_ func(pid, sig uint32) error, _ any) ProcessKillerOS {
	return &ProcessKillerWindows{}
}

// Kill tries to kill from userspace
func (p *ProcessKillerWindows) Kill(sig uint32, pc *killContext) error {
	if sig != model.SIGKILL {
		return nil
	}
	return winutil.KillProcess(int(pc.pid), 0)
}

// KillCgroup is not supported on Windows
func (p *ProcessKillerWindows) KillCgroup(_ cgroupKillTarget) error {
	return errors.New("cgroups are not supported")
}

// getCgroupKillTarget always reports that killing a cgroup at once isn't possible on Windows
func (p *ProcessKillerWindows) getCgroupKillTarget(_ string, _ *model.ProcessCacheEntry) (cgroupKillTarget, bool) {
	return cgroupKillTarget{}, false
}

func (p *ProcessKillerWindows) getProcesses(scope string, ev *model.Event, _ *model.ProcessCacheEntry) ([]killContext, error) {
	if scope == "container" {
		return nil, errors.New("container scope not supported")
	}
	return []killContext{
		{
			pid:  int(ev.ProcessContext.Pid),
			path: ev.ProcessContext.FileEvent.PathnameStr,
		},
	}, nil
}
