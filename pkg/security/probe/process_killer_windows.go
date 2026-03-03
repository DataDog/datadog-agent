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
