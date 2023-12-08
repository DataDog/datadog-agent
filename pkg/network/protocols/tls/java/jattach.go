// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package java

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process"
)

// minJavaAgeToAttachMS is the minimum age of a java process to be able to attach it
// else the java process would crash if he receives the SIGQUIT too early ("Signal Dispatch" thread is not ready)
// In other words that the only reliable safety thing we could check to assume a java process started (System.main execution)
// Looking a proc/pid/status.Thread numbers is not reliable as it depend on numbers of cores and JRE version/implementation
//
// The issue is described here https://bugs.openjdk.org/browse/JDK-8186709 see Kevin Walls comment
// if java received a SIGQUIT and the JVM is not started yet, java will print 'quit (core dumped)'
// SIGQUIT is sent as part of the hotspot protocol handshake
const minJavaAgeToAttachMS = 10000

func injectAttach(pid int, agent, args string, nsPid, fsUID, fsGID int) error {
	h, err := NewHotspot(pid, nsPid)
	if err != nil {
		return err
	}

	return h.Attach(agent, args, fsUID, fsGID)
}

// InjectAgent injects the given agent into the given java process
func InjectAgent(pid int, agent, args string) error {
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
	// we return the process uid and gid from the filesystem point of view
	// as attach file need to be created with uid/gid accessible from the java hotspot
	// index 3 here point to the 4th columns of /proc/pid/status Uid/Gid => filesystem uid/gid
	fsUID, fsGID := int(uids[3]), int(gids[3])

	ctime, _ := proc.CreateTime()
	ageMs := time.Now().UnixMilli() - ctime
	if ageMs < minJavaAgeToAttachMS {
		log.Debugf("java attach pid %d will be delayed by %d ms", pid, minJavaAgeToAttachMS-ageMs)
		// wait and inject the agent asynchronously
		go func() {
			time.Sleep(time.Duration(minJavaAgeToAttachMS-ageMs) * time.Millisecond)
			if err := injectAttach(pid, agent, args, int(proc.NsPid), fsUID, fsGID); err != nil {
				log.Errorf("java attach pid %d failed %s", pid, err)
			}
		}()
		return nil
	}

	return injectAttach(pid, agent, args, int(proc.NsPid), fsUID, fsGID)
}
