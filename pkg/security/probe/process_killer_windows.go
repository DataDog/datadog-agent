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

// KillFromUserspace tries to kill from userspace
func (p *ProcessKiller) KillFromUserspace(pid uint32, sig uint32, _ *model.Event) error {
	if sig != model.SIGKILL {
		return nil
	}
	return winutil.KillProcess(int(pid), 0)
}

func (p *ProcessKiller) getPids(scope string, ev *model.Event, _ *model.ProcessCacheEntry) ([]uint32, error) {
	if scope == "container" {
		return nil, errors.New("container scope not supported")
	}
	return []uint32{ev.ProcessContext.Pid}, nil
}
