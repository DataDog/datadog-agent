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

	// we return the process uid and gid from the filesystem point of view
	// as attach file need to be created with uid/gid accessible from the java hotspot
	// index 3 here point to the 4th columns of /proc/pid/status Uid/Gid => filesystem uid/gid
	return h.Attach(agent, args, int(uids[3]), int(gids[3]))
}
