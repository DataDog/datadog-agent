// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/prometheus/procfs"
	"tailscale.com/portlist"
)

func (c *Check) discoverServices() error {
	processes, err := procfs.AllProcs()
	if err != nil {
		return err
	}

	log.Infof("alive: %d | services: %d | ignore: %d", len(processes), len(c.services.m), len(c.ignore.m))
	aliveProcesses := make(map[int]struct{})

	var wg sync.WaitGroup

	for _, p := range processes {
		aliveProcesses[p.PID] = struct{}{}
		name, shortName, err := processName(p)
		if err != nil {
			log.Errorf("[pid: %d] failed to get process name: %v", p.PID, err)
			continue
		}

		ignore := false
		if ignoreName, ok := c.ignore.get(p.PID); ok {
			// ignore
			if ignoreName == name {
				ignore = true
			} else {
				// same pid but different process name, this means the pid has been reused for
				// a different process
				c.ignore.delete(p.PID)
			}
		} else if _, ok := c.alwaysIgnore[shortName]; ok {
			ignore = true
			c.ignore.set(p.PID, name)
		}

		if !ignore {
			// process is a potential service
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.scanProcess(p.PID)
			}()
		}
	}

	for pid, pInfo := range c.services.m {
		_, ok := aliveProcesses[pid]
		if !ok {
			c.sendEndServiceEvent(pInfo)
			c.services.delete(pid)
		}
	}

	// cleanup ignore processes that are no longer alive
	for pid, _ := range c.ignore.m {
		_, ok := aliveProcesses[pid]
		if !ok {
			c.ignore.delete(pid)
		}
	}

	wg.Wait()
	return nil
}

func (c *Check) scanProcess(pid int) {
	p, err := procfs.NewProc(pid)
	if err != nil {
		log.Errorf("failed to get proc: %v", err)
		return
	}

	if pInfo, ok := c.services.get(pid); ok {
		if time.Since(pInfo.LastHeatBeatTime) >= heartbeatTime {
			c.sendHeartbeatServiceEvent(pInfo)
			pInfo.LastHeatBeatTime = time.Now()
		}
		return
	}

	pInfo, err := getProcessInfo(p)
	if err != nil {
		log.Errorf("failed to get process info: %v", err)
		return
	}

	if len(pInfo.Ports) == 0 {
		log.Infof("[pid: %d | name: %s]: process has no open ports, ignoring...", pid, pInfo.ShortName)
		c.ignore.set(pid, pInfo.Name)
		return
	}

	c.services.set(pid, pInfo)
	c.sendStartServiceEvent(pInfo)
}

/*
- command line /proc/{pid}/cmdline
- environment variables /proc/{pid}/environ
- CWD (PWD env var)
- open ports /proc/{pid}/net/tcp|udp|tcp6|udp6
- time launched /proc/{pid}/stat
*/
func getProcessInfo(p procfs.Proc) (*processInfo, error) {
	cmdline, err := p.CmdLine()
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/{pid}/cmdline: %w", err)
	}

	env, err := p.Environ()
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/{pid}/environ: %w", err)
	}

	cwd, err := p.Cwd()
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/{pid}/cwd: %w", err)
	}

	stat, err := p.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/{pid}/stat: %w", err)
	}

	var poller portlist.Poller

	pl, _, err := poller.Poll()
	if err != nil {
		return nil, err
	}

	var processPorts []int
	for _, port := range pl {
		if port.Pid == p.PID {
			processPorts = append(processPorts, int(port.Port))
		}
	}

	return &processInfo{
		PID:              p.PID,
		Name:             nameFromArgv(cmdline),
		ShortName:        shortNameFromArgv(cmdline),
		CmdLine:          cmdline,
		Env:              env,
		Cwd:              cwd,
		Stat:             &stat,
		Ports:            processPorts,
		DetectedTime:     time.Now(),
		LastHeatBeatTime: time.Now(),
	}, nil
}

func processName(p procfs.Proc) (string, string, error) {
	cmdline, err := p.CmdLine()
	if err != nil {
		return "", "", fmt.Errorf("failed to read /proc/{pid}/cmdline: %w", err)
	}

	return nameFromArgv(cmdline), shortNameFromArgv(cmdline), nil
}

func nameFromArgv(argv []string) string {
	return strings.Join(argv, " ")
}

// argvSubject takes a command and its flags, and returns the
// short/pretty name for the process.
func shortNameFromArgv(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	ret := filepath.Base(argv[0])

	// Handle special cases.
	switch {
	case ret == "mono" && len(argv) >= 2:
		// .Net programs execute as `mono actualProgram.exe`.
		ret = filepath.Base(argv[1])
	}

	// Handle space separated argv
	ret, _, _ = strings.Cut(ret, " ")

	// Remove common noise.
	ret = strings.TrimSpace(ret)
	ret = strings.TrimSuffix(ret, ".exe")

	return ret
}
