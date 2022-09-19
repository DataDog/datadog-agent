// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package java

import (
	"github.com/DataDog/gopsutil/process"
)

func InjectAgent(pid int, agent string, args string) error {
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return err
	}
	uids, err := proc.Uids()
	if err != nil {
		return err
	}
	gids, err := proc.Gids()
	if err != nil {
		return err
	}

	// attach
	h, err := NewHotspot(pid, int(proc.NsPid))
	if err != nil {
		return err
	}

	err = h.Attach(agent, args, int(uids[3]), int(gids[3])) // 3: filesystem uid/gid
	if err != nil {
		return err
	}
	return nil
}
