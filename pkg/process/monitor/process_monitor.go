// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package monitor

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"sync"

	"github.com/vishvananda/netlink"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process"
)

const (
	processMonitorMaxEvents = 2048
)

var (
	processMonitorLock sync.RWMutex
	processMonitor     *ProcessMonitor
)

type ProcessMonitor struct {
	m  sync.Mutex
	wg sync.WaitGroup

	initializing bool

	events chan netlink.ProcEvent
	done   chan struct{}
	errors chan error

	procEventCallbacks map[ProcessEventType][]*ProcessCallback
	callbackRunner     chan func()
	callbackRunnerDone chan struct{}
}

type ProcessEventType int

const (
	EXEC ProcessEventType = iota
	FORK
	EXIT
)

type ProcessMetadataField int

const (
	ANY ProcessMetadataField = iota
	NAME
	MAPFILE
)

type ProcessCallback struct {
	Event    ProcessEventType
	Metadata ProcessMetadataField
	Regex    *regexp.Regexp
	Callback func(pid uint32)
}

func (pm *ProcessMonitor) enqueueCallback(callback *ProcessCallback, pid uint32) {
	pm.callbackRunner <- func() { callback.Callback(pid) }
}

// Initialize() will scan all running processes and execute matching callbacks
// Once it's done all new events from netlink socket will be processed by the main async loop
func (pm *ProcessMonitor) Initialize() error {
	fn := func(pid int) error {
		for _, c := range pm.procEventCallbacks[EXEC] {
			pm.evalEXECCallback(c, uint32(pid))
		}
		return nil
	}

	pm.m.Lock()
	defer pm.m.Unlock()
	if err := util.WithAllProcs(util.HostProc(), fn); err != nil {
		return fmt.Errorf("process monitor init, scanning all process failed %s", err)
	}
	// enable events to be processed
	pm.initializing = false
	return nil
}

func (p *ProcessMonitor) evalEXECCallback(c *ProcessCallback, pid uint32) {
	if c.Metadata == ANY {
		p.enqueueCallback(c, pid)
		return
	}

	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		log.Errorf("process %d parsing failed %s", pid, err)
		return
	}
	switch c.Metadata {
	case NAME:
		pname, err := proc.Name()
		if err != nil {
			log.Errorf("process %d name parsing failed %s", pid, err)
			return
		}
		if c.Regex.MatchString(pname) {
			p.enqueueCallback(c, pid)
		}
	case MAPFILE:
		mmaps, err := proc.MemoryMaps(true)
		if err != nil {
			log.Errorf("process %d maps parsing failed %s", pid, err)
			return
		}
		if mmaps == nil {
			return
		}
		for _, mmap := range *mmaps {
			if c.Regex.MatchString(mmap.Path) {
				p.enqueueCallback(c, pid)
				break
			}
		}
	}
}

func (pm *ProcessMonitor) Subscribe(callback *ProcessCallback) (UnSubscribe func(), err error) {
	pm.m.Lock()
	defer pm.m.Unlock()

	for _, c := range pm.procEventCallbacks[callback.Event] {
		if c == callback {
			return func() {}, errors.New("same callback can't be registred twice")
		}
	}

	pm.procEventCallbacks[callback.Event] = append(pm.procEventCallbacks[callback.Event], callback)

	// UnSubscribe()
	return func() {
		pm.m.Lock()
		defer pm.m.Unlock()

		for i, c := range pm.procEventCallbacks[callback.Event] {
			if c == callback {
				l := len(pm.procEventCallbacks[callback.Event])
				pm.procEventCallbacks[callback.Event][i] = pm.procEventCallbacks[callback.Event][l-1]
				pm.procEventCallbacks[callback.Event] = pm.procEventCallbacks[callback.Event][:l-1]
				return
			}
		}
	}, nil
}

func (pm *ProcessMonitor) Stop() {
	close(pm.callbackRunnerDone)
	close(pm.done)
	pm.wg.Wait()

	processMonitorLock.Lock()
	processMonitor = nil
	processMonitorLock.Unlock()
}

// GetProcessMonitor() create a monitor (only once) that register to netlink process events.
//
// This monitor can monitor.Subscribe(callback, filter) callback on particual event
// like process EXEC, EXIT. The callback will be called when the filter will match.
// Filter can be applied on :
//   process name (NAME)
//   /proc/pid/smaps file (MAPFILE)
//   by default ANY is applied
//
// Typical initialization:
//   mon, _ := GetProcessMonitor()
//   mon.Subcribe(callback)
//   mon.Initialize()
//
// note: o GetProcessMonitor() will always return the same instance
//         as we can only regiter once with netlink process event
//       o mon.Subcribe() will subscribe callback before or after the Initialization
//       o mon.Initialize() will scan current processes and call subscribed callback
//
func GetProcessMonitor() (*ProcessMonitor, error) {
	processMonitorLock.RLock()
	if processMonitor != nil {
		defer processMonitorLock.RUnlock()
		return processMonitor, nil
	}
	processMonitorLock.RUnlock()

	processMonitorLock.Lock()
	defer processMonitorLock.Unlock()

	p := &ProcessMonitor{}
	p.initializing = true
	p.events = make(chan netlink.ProcEvent, processMonitorMaxEvents)
	p.done = make(chan struct{})
	p.errors = make(chan error)
	p.procEventCallbacks = make(map[ProcessEventType][]*ProcessCallback)

	if err := netlink.ProcEventMonitor(p.events, p.done, p.errors); err != nil {
		return nil, fmt.Errorf("could not create process monitor: %s", err)
	}

	p.callbackRunnerDone = make(chan struct{}, runtime.NumCPU())
	p.callbackRunner = make(chan func(), runtime.NumCPU())
	for i := 0; i < runtime.NumCPU(); i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-p.callbackRunnerDone:
					return
				case call, ok := <-p.callbackRunner:
					if !ok {
						continue
					}
					call()
				}
			}
		}()
	}

	// This is the main async loop, where we process processes events from netlink socket
	// events are dropped until
	p.wg.Add(1)
	go func() {
		defer func() {
			log.Info("netlink process monitor ended")
			p.wg.Done()
		}()
		for {
			select {
			case <-p.done:
				return

			case event, ok := <-p.events:
				if !ok {
					return
				}
				p.m.Lock()
				if p.initializing {
					p.m.Unlock()
					continue
				}

				switch ev := event.Msg.(type) {
				case *netlink.ExecProcEvent:
					for _, c := range p.procEventCallbacks[EXEC] {
						p.evalEXECCallback(c, ev.ProcessPid)
					}
				case *netlink.ExitProcEvent:
					for _, c := range p.procEventCallbacks[EXIT] {
						p.enqueueCallback(c, ev.ProcessPid)
					}
				}
				p.m.Unlock()

			case err, ok := <-p.errors:
				if !ok {
					return
				}
				log.Errorf("process montior error: %s", err)
				p.Stop()
				return
			}
		}
	}()

	processMonitor = p
	return p, nil
}
