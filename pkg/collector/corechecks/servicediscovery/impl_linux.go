// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"fmt"
	"time"

	"github.com/prometheus/procfs"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/portlist"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=impl_linux_mock.go

func init() {
	newOSImpl = newLinuxImpl
}

var ignoreCfgLinux = []string{
	"sshd",
	"dhclient",
	"systemd",
	"systemd-resolved",
	"systemd-networkd",
	"datadog-agent",
	"livenessprobe",
	"docker-proxy", // remove when we have docker support in place
}

type linuxImpl struct {
	procfs     procFS
	portPoller portPoller
	time       timer

	serviceDetector *serviceDetector
	sender          *telemetrySender
	ignoreCfg       map[string]bool

	ignoreProcs       map[int]bool
	aliveServices     map[int]*serviceInfo
	potentialServices map[int]*serviceInfo
}

func newLinuxImpl(sender *telemetrySender, ignoreCfg map[string]bool) (osImpl, error) {
	for _, i := range ignoreCfgLinux {
		ignoreCfg[i] = true
	}
	pfs, err := procfs.NewDefaultFS()
	if err != nil {
		return nil, err
	}
	poller, err := portlist.NewPoller()
	if err != nil {
		return nil, err
	}
	return &linuxImpl{
		procfs:            wProcFS{pfs},
		portPoller:        poller,
		time:              realTime{},
		sender:            sender,
		serviceDetector:   newServiceDetector(),
		ignoreCfg:         ignoreCfg,
		ignoreProcs:       make(map[int]bool),
		aliveServices:     make(map[int]*serviceInfo),
		potentialServices: make(map[int]*serviceInfo),
	}, nil
}

type processEvents struct {
	started []serviceInfo
	stopped []serviceInfo
}

type eventsByName map[string]*processEvents

func (e eventsByName) addStarted(svc serviceInfo) {
	events, ok := e[svc.meta.Name]
	if !ok {
		events = &processEvents{}
	}
	events.started = append(events.started, svc)
	e[svc.meta.Name] = events
}

func (e eventsByName) addStopped(svc serviceInfo) {
	events, ok := e[svc.meta.Name]
	if !ok {
		events = &processEvents{}
	}
	events.stopped = append(events.stopped, svc)
	e[svc.meta.Name] = events
}

func (li *linuxImpl) DiscoverServices() error {
	procs, err := li.aliveProcs()
	if err != nil {
		return fmt.Errorf("failed to get alive processes: %w", err)
	}

	ports, err := li.openPorts()
	if err != nil {
		return fmt.Errorf("failed to get open ports: %w", err)
	}

	log.Debugf("aliveProcs: %d | ignoreProcs: %d | runningServices: %d | potentials: %d | openPorts: %+v",
		len(procs),
		len(li.ignoreProcs),
		len(li.aliveServices),
		len(li.potentialServices),
		ports,
	)

	var (
		started []serviceInfo
		stopped []serviceInfo
	)

	// potentialServices contains processes that we scanned in the previous iteration and had open ports.
	// we check if they are still alive in this iteration, and if so, we send a start-service telemetry event.
	for pid, svc := range li.potentialServices {
		if _, ok := procs[pid]; ok {
			li.aliveServices[pid] = svc
			started = append(started, *svc)
		}
	}
	clear(li.potentialServices)

	// check open ports - these will be potential new services if they are still alive in the next iteration.
	for pid := range ports {
		if li.ignoreProcs[pid] {
			continue
		}
		if _, ok := li.aliveServices[pid]; !ok {
			log.Debugf("[pid: %d] found new process with open ports", pid)

			p, ok := procs[pid]
			if !ok {
				log.Debugf("[pid: %d] process with open ports was not found in alive procs", pid)
				continue
			}

			svc, err := li.getServiceInfo(p, ports)
			if err != nil {
				log.Errorf("[pid: %d] failed to get process info: %v", pid, err)
				li.ignoreProcs[pid] = true
				continue
			}
			if li.ignoreCfg[svc.meta.Name] {
				log.Debugf("[pid: %d] process ignored from config: %s", pid, svc.meta.Name)
				li.ignoreProcs[pid] = true
				continue
			}
			log.Debugf("[pid: %d] adding process to potential: %s", pid, svc.meta.Name)
			li.potentialServices[pid] = svc
		}
	}

	// check if services previously marked as alive still are.
	now := li.time.Now()
	for pid, svc := range li.aliveServices {
		if _, ok := procs[pid]; !ok {
			delete(li.aliveServices, pid)
			stopped = append(stopped, *svc)
		} else if now.Sub(svc.LastHeartbeat).Truncate(time.Minute) >= heartbeatTime {
			li.sender.sendHeartbeatServiceEvent(*svc)
			svc.LastHeartbeat = now
		}
	}

	// check if services previously marked as ignore are still alive.
	for pid := range li.ignoreProcs {
		if _, ok := procs[pid]; !ok {
			delete(li.ignoreProcs, pid)
		}
	}

	// group started and stopped processes by name
	events := make(eventsByName)
	for _, p := range started {
		events.addStarted(p)
	}
	for _, p := range stopped {
		events.addStopped(p)
	}

	potentialNames := map[string]bool{}
	for _, p := range li.potentialServices {
		potentialNames[p.meta.Name] = true
	}

	for name, ev := range events {
		if len(ev.started) > 0 && len(ev.stopped) > 0 {
			log.Warnf("found multiple started/stopped processes with the same name, ignoring end-service events (name: %q)", name)
			clear(ev.stopped)
		}
		for _, svc := range ev.started {
			li.sender.sendStartServiceEvent(svc)
		}
		for _, svc := range ev.stopped {
			if potentialNames[name] {
				log.Debugf("there is a potential service with the same name as a stopped one, skipping end-service event (name: %q)", name)
				break
			}
			li.sender.sendEndServiceEvent(svc)
		}
	}

	return nil
}

func (li *linuxImpl) aliveProcs() (map[int]proc, error) {
	procs, err := li.procfs.AllProcs()
	if err != nil {
		return nil, err
	}
	procMap := map[int]proc{}
	for _, v := range procs {
		procMap[v.PID()] = v
	}
	return procMap, nil
}

func (li *linuxImpl) openPorts() (map[int]portlist.List, error) {
	ports, err := li.portPoller.OpenPorts()
	if err != nil {
		return nil, err
	}
	portMap := map[int]portlist.List{}
	for _, p := range ports {
		portMap[p.Pid] = append(portMap[p.Pid], p)
	}
	return portMap, nil
}

func (li *linuxImpl) getServiceInfo(p proc, openPorts map[int]portlist.List) (*serviceInfo, error) {
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
	for _, port := range openPorts[p.PID()] {
		ports = append(ports, int(port.Port))
	}

	// if the process name is docker-proxy, we should talk to docker to get the process command line and env vars
	// have to see how far this can go but not for the initial release

	// for now, docker-proxy is going on the ignore list

	pInfo := processInfo{
		PID:     p.PID(),
		CmdLine: cmdline,
		Env:     env,
		Cwd:     cwd,
		Stat: procStat{
			StartTime: stat.Starttime,
		},
		Ports: ports,
	}

	meta := li.serviceDetector.Detect(pInfo)

	return &serviceInfo{
		process:       pInfo,
		meta:          meta,
		LastHeartbeat: li.time.Now(),
	}, nil
}

type proc interface {
	PID() int
	CmdLine() ([]string, error)
	Environ() ([]string, error)
	Cwd() (string, error)
	Stat() (procfs.ProcStat, error)
}

type wProc struct {
	procfs.Proc
}

func (w wProc) PID() int {
	return w.Proc.PID
}

type procFS interface {
	AllProcs() ([]proc, error)
}

type portPoller interface {
	OpenPorts() (portlist.List, error)
}

type wProcFS struct {
	procfs.FS
}

func (w wProcFS) AllProcs() ([]proc, error) {
	procs, err := w.FS.AllProcs()
	if err != nil {
		return nil, err
	}
	var res []proc
	for _, p := range procs {
		res = append(res, wProc{p})
	}
	return res, nil
}
