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

	"github.com/prometheus/procfs"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicedetector"
	"github.com/DataDog/datadog-agent/pkg/process/portlist"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	newOSImpl = newLinuxImpl
}

type aliveMap struct {
	m  map[int]*processInfo
	mu sync.RWMutex
}

func (a *aliveMap) get(pid int) (*processInfo, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	v, ok := a.m[pid]
	return v, ok
}

func (a *aliveMap) set(pid int, info *processInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m[pid] = info
}

func (a *aliveMap) delete(pid int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.m, pid)
}

type ignoreMap struct {
	m  map[int]string
	mu sync.RWMutex
}

func (i *ignoreMap) get(pid int) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	v, ok := i.m[pid]
	return v, ok
}

func (i *ignoreMap) set(pid int, name string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.m[pid] = name
}

func (i *ignoreMap) delete(pid int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.m, pid)
}

type linuxImpl struct {
	sender    *telemetrySender
	ignoreCfg map[string]struct{}

	// PID -> process info
	// pids that are considered services will be added here.
	alive *aliveMap

	// PID -> process name
	// pids will be added here because the name was ignored in the config, or because
	// it was already scanned and had no open ports.
	ignore *ignoreMap
}

func newLinuxImpl(sender *telemetrySender, ignoreProcesses map[string]struct{}) osImpl {
	return &linuxImpl{
		sender:    sender,
		ignoreCfg: ignoreProcesses,
		ignore: &ignoreMap{
			m: make(map[int]string),
		},
		alive: &aliveMap{
			m: make(map[int]*processInfo),
		},
	}
}

func (i *linuxImpl) DiscoverServices() error {
	processes, err := procfs.AllProcs()
	if err != nil {
		return err
	}

	portPoller, err := portlist.NewPoller(false)
	if err != nil {
		return fmt.Errorf("failed to initialize port poller: %w", err)
	}
	defer func() {
		if err := portPoller.Close(); err != nil {
			log.Warnf("failed to close port poller: %v", err)
		}
	}()

	openPorts, err := portPoller.ListeningPorts()
	if err != nil {
		return err
	}

	log.Infof("alive: %d | services: %d | ignore: %d", len(processes), len(i.alive.m), len(i.ignore.m))
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
		if ignoreName, ok := i.ignore.get(p.PID); ok {
			// ignore
			if ignoreName == name {
				ignore = true
			} else {
				// same pid but different process name, this means the pid has been reused for
				// a different process
				i.ignore.delete(p.PID)
			}
		} else if _, ok := i.ignoreCfg[shortName]; ok {
			ignore = true
			i.ignore.set(p.PID, name)
		}

		if !ignore {
			// process is a potential service
			wg.Add(1)
			go func() {
				defer wg.Done()
				i.scanProcess(p.PID, openPorts)
			}()
		}
	}

	for pid, pInfo := range i.alive.m {
		_, ok := aliveProcesses[pid]
		if !ok {
			// TODO: if there is a start and a stop for the same process name in this iteration, it means the process
			// 	has been restarted. In that case we should only send one of the events.
			i.sender.sendEndServiceEvent(pInfo)
			i.alive.delete(pid)
		}
	}

	// cleanup ignore processes that are no longer alive
	for pid, _ := range i.ignore.m {
		_, ok := aliveProcesses[pid]
		if !ok {
			i.ignore.delete(pid)
		}
	}

	wg.Wait()
	return nil
}

func (i *linuxImpl) scanProcess(pid int, openPorts portlist.List) {
	p, err := procfs.NewProc(pid)
	if err != nil {
		log.Errorf("failed to get proc: %v", err)
		return
	}

	if pInfo, ok := i.alive.get(pid); ok {
		if time.Since(pInfo.LastHeatBeatTime) >= heartbeatTime {
			i.sender.sendHeartbeatServiceEvent(pInfo)
			pInfo.LastHeatBeatTime = time.Now()
		}
		return
	}

	pInfo, err := getProcessInfo(p, openPorts)
	if err != nil {
		log.Errorf("failed to get process info: %v", err)
		return
	}

	if len(pInfo.Ports) == 0 {
		log.Infof("[pid: %d | name: %s]: process has no open ports, ignoring...", pid, pInfo.ShortName)
		i.ignore.set(pid, pInfo.Name)
		return
	}

	dCtx := servicedetector.New(pInfo.CmdLine, pInfo.Env)
	meta, ok := dCtx.Detect()
	if !ok {
		log.Warnf("failed to detect service (pid: %d, name: %s)", pInfo.PID, pInfo.ShortName)
		return
	}

	if len(meta.AdditionalNames) > 0 {
		for _, n := range meta.AdditionalNames {
			pInfo.Services = append(pInfo.Services, &serviceInfo{Name: n})
		}
	} else {
		pInfo.Services = append(pInfo.Services, &serviceInfo{Name: meta.Name})
	}

	i.alive.set(pid, pInfo)
	i.sender.sendStartServiceEvent(pInfo)
}

/*
- command line /proc/{pid}/cmdline
- environment variables /proc/{pid}/environ
- CWD (PWD env var)
- open ports /proc/{pid}/net/tcp|udp|tcp6|udp6
- time launched /proc/{pid}/stat
*/
func getProcessInfo(p procfs.Proc, openPorts portlist.List) (*processInfo, error) {
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

	var processPorts []int
	for _, port := range openPorts {
		if port.Pid == p.PID {
			processPorts = append(processPorts, int(port.Port))
		}
	}

	return &processInfo{
		PID:       p.PID,
		Name:      nameFromArgv(cmdline),
		ShortName: shortNameFromArgv(cmdline),
		CmdLine:   cmdline,
		Env:       env,
		Cwd:       cwd,
		Services:  nil,
		Stat: &procStat{
			StartTime: stat.Starttime,
		},
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
