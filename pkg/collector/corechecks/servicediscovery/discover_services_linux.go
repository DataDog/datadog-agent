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
	"time"

	"github.com/prometheus/procfs"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicedetector"
	"github.com/DataDog/datadog-agent/pkg/process/portlist"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	newOSImpl = newLinuxImpl
}

var ignoreCfgLinux = []string{
	"sshd",
	"dhclient",
}

type linuxImpl struct {
	sender    *telemetrySender
	ignoreCfg map[string]bool

	ignoreProcs       map[int]bool
	aliveServices     map[int]*processInfo
	potentialServices map[int]*processInfo
}

func newLinuxImpl(sender *telemetrySender, ignoreCfg map[string]bool) osImpl {
	for _, i := range ignoreCfgLinux {
		ignoreCfg[i] = true
	}
	return &linuxImpl{
		sender:            sender,
		ignoreCfg:         ignoreCfg,
		ignoreProcs:       make(map[int]bool),
		aliveServices:     make(map[int]*processInfo),
		potentialServices: make(map[int]*processInfo),
	}
}

type processEvents struct {
	started []*processInfo
	stopped []*processInfo
}

type eventsByName map[string]*processEvents

func (e eventsByName) getOrCreate(name string) *processEvents {
	events, ok := e[name]
	if ok {
		return events
	}
	e[name] = &processEvents{}
	return e[name]
}

func (e eventsByName) addStarted(p *processInfo) {
	events := e.getOrCreate(p.Service.Name)
	events.started = append(events.started, p)
}

func (e eventsByName) addStopped(p *processInfo) {
	events := e.getOrCreate(p.Service.Name)
	events.stopped = append(events.stopped, p)
}

func (i *linuxImpl) DiscoverServices() error {
	procs, err := i.aliveProcs()
	if err != nil {
		return fmt.Errorf("failed to get alive processes: %w", err)
	}

	ports, err := i.openPorts()
	if err != nil {
		return fmt.Errorf("failed to get get ports: %w", err)
	}

	log.Infof("aliveProcs: %d | ignoreProcs: %d | runningServices: %d | openPorts: %v",
		len(procs),
		len(i.ignoreProcs),
		len(i.aliveServices),
		ports,
	)

	var (
		started []*processInfo
		stopped []*processInfo
	)

	// potentialServices contains processes that we scanned in the previous iteration and had open ports.
	// we check if they are still alive in this iteration, and if so, we send a start-service telemetry event.
	for pid, _ := range i.potentialServices {
		if proc, ok := procs[pid]; ok {
			pInfo, err := getProcessInfo(proc, ports)
			if err != nil {
				log.Errorf("[pid: %d] failed to get process info: %v", pid, err)
				continue
			}
			started = append(started, pInfo)
		}
	}
	clear(i.potentialServices)

	// check open ports - these will be potential new services if they are still alive in the next iteration.
	for pid, _ := range ports {
		if i.ignoreProcs[pid] {
			continue
		}
		if _, ok := i.aliveServices[pid]; !ok {
			proc := procs[pid]
			pInfo, err := getProcessInfo(proc, ports)
			if err != nil {
				log.Errorf("[pid: %d] failed to get process info: %v", pid, err)
				continue
			}
			if i.ignoreCfg[pInfo.Service.Name] {
				log.Infof("[pid: %d] process ignored from config: %s", pid, pInfo.Service.Name)
				i.ignoreProcs[pid] = true
				continue
			}
			i.potentialServices[pid] = pInfo
		}
	}

	// check if services previously marked as alive still are.
	for pid, pInfo := range i.aliveServices {
		if _, ok := procs[pid]; !ok {
			stopped = append(stopped, pInfo)
		} else if time.Since(pInfo.LastHeartBeatTime) >= heartbeatTime {
			i.sender.sendHeartbeatServiceEvent(pInfo)
			pInfo.LastHeartBeatTime = time.Now()
		}
	}

	// check if services previously marked as ignore are still alive.
	for pid, _ := range i.ignoreProcs {
		if _, ok := procs[pid]; !ok {
			delete(i.ignoreProcs, pid)
		}
	}

	// group started and stopped processes by name
	events := make(eventsByName)
	for _, p := range started {
		i.aliveServices[p.PID] = p
		events.addStarted(p)
	}
	for _, p := range stopped {
		delete(i.aliveServices, p.PID)
		events.addStopped(p)
	}

	for name, ev := range events {
		if len(ev.started) > 0 && len(ev.stopped) > 0 {
			// TODO: do something smarter here
			log.Warnf("found multiple started/stopped processes with the same name, skipping events (name: %q)", name)
			continue
		}
		for _, p := range ev.started {
			i.sender.sendStartServiceEvent(p)
		}
		for _, p := range ev.stopped {
			i.sender.sendEndServiceEvent(p)
		}
	}

	return nil
}

func (i *linuxImpl) aliveProcs() (map[int]procfs.Proc, error) {
	processes, err := procfs.AllProcs()
	if err != nil {
		return nil, err
	}
	procMap := map[int]procfs.Proc{}
	for _, v := range processes {
		procMap[v.PID] = v
	}
	return procMap, nil
}

func (i *linuxImpl) openPorts() (map[int]portlist.List, error) {
	poller, err := portlist.NewPoller(false)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize port poller: %w", err)
	}
	defer func() {
		if err := poller.Close(); err != nil {
			log.Warnf("failed to close port poller: %v", err)
		}
	}()

	ports, err := poller.ListeningPorts()
	if err != nil {
		return nil, err
	}

	portMap := map[int]portlist.List{}
	for _, p := range ports {
		portMap[p.Pid] = append(portMap[p.Pid], p)
	}
	return portMap, nil
}

/*
- command line /proc/{pid}/cmdline
- environment variables /proc/{pid}/environ
- CWD (PWD env var)
- open ports /proc/{pid}/net/tcp|udp|tcp6|udp6
- time launched /proc/{pid}/stat
*/
func getProcessInfo(p procfs.Proc, openPorts map[int]portlist.List) (*processInfo, error) {
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

	var ports []int
	for _, port := range openPorts[p.PID] {
		ports = append(ports, int(port.Port))
	}

	dCtx := servicedetector.New(cmdline, env)
	svcMeta := dCtx.Detect()
	log.Infof("servicedetector returned metadata: %v (cmdline: %v, env: %v)", svcMeta, cmdline, env)

	return &processInfo{
		PID:     p.PID,
		CmdLine: cmdline,
		Env:     env,
		Cwd:     cwd,
		Service: serviceInfo{
			Name:     svcMeta.Name,
			Language: 0, // TODO
			Type:     0, // TODO
		},
		Stat: procStat{
			StartTime: stat.Starttime,
		},
		Ports:             ports,
		DetectedTime:      time.Now(),
		LastHeartBeatTime: time.Now(),
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
