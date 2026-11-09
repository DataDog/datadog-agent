// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"errors"

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

// ProcessKillerWindows defines the process kill windows implementation
type ProcessKillerWindows struct{}

// NewProcessKillerOS returns a ProcessKillerOS
// The last two parameters are cgroup related and ignored on Windows, as container scope is not supported
func NewProcessKillerOS(_ func(pid, sig uint32) error, _ any, _ bool) ProcessKillerOS {
	return &ProcessKillerWindows{}
}

// Kill tries to kill the given processes from userspace, one by one
func (p *ProcessKillerWindows) Kill(sig uint32, kcs []killContext) ([]uint32, []uint32) {
	var failedPids, killedPids []uint32

	for _, kc := range kcs {
		if sig == model.SIGKILL {
			if err := winutil.KillProcess(kc.pid, 0); err != nil {
				failedPids = append(failedPids, uint32(kc.pid))
				continue
			}
		}
		killedPids = append(killedPids, uint32(kc.pid))
	}

	return failedPids, killedPids
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
