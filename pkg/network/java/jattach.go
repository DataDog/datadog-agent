// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package java

import (
	"fmt"
	"time"

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

	end := time.Now().Add(5 * time.Second)
	for end.After(time.Now()) {
		// wait the java process start the JVM
		// issue describe here https://bugs.openjdk.org/browse/JDK-8186709 see Kevin Walls comment
		// if java received a SIGQUIT and the JVM is not started yet, java will print 'quit (core dumped)'
		// SIGQUIT is sent as part of the hotspot protocol handshake
		// JVM Threads : "VM Thread", "Reference Handl", "Finalizer", "Signal Dispatch"
		// "Signal Dispatch" is thread number 19 (x86_64 openjdk 1.8.0_352), so a new magic number ;)
		nThreads, err := proc.NumThreads()
		if err != nil {
			return err
		}
		if nThreads > 19 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if time.Now().After(end) {
		ctime, errctime := proc.CreateTime()
		nThreads, err := proc.NumThreads()
		return fmt.Errorf("java process %d didn't start in time, can't inject the agent : timeout %v %v   %v %v", pid, nThreads, err, time.Now().Unix()-ctime, errctime)
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
