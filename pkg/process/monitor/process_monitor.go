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
	"time"

	"github.com/DataDog/gopsutil/process"
	"github.com/vishvananda/netlink"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	processMonitorMaxEvents = 2048
)

var (
	once           sync.Once
	processMonitor *ProcessMonitor
)

// ProcessMonitor will subscribe to the netlink process events like Exec, Exit
// and call the subscribed callbacks
// Initialize() will scan the current process and will call the subscribed callbacks
//
// callbacks will be executed in parallel via a pool of goroutines (runtime.NumCPU())
// callbackRunner is callbacks queue. The queue size is set by processMonitorMaxEvents
type ProcessMonitor struct {
	m  sync.Mutex
	wg sync.WaitGroup

	isInitialized bool

	// chan push done by vishvananda/netlink library
	events chan netlink.ProcEvent
	done   chan struct{}
	errors chan error

	// callback registration and parallel execution management
	procEventCallbacks map[ProcessEventType][]*ProcessCallback
	runningPids        map[uint32]struct{}
	callbackRunner     chan func()
	callbackRunnerDone chan struct{}
}

type ProcessEventType int

const (
	EXEC ProcessEventType = iota
	EXIT
)

type ProcessMetadataField int

const (
	ANY ProcessMetadataField = iota
	NAME
)

type ProcessCallback struct {
	Event    ProcessEventType
	Metadata ProcessMetadataField
	Regex    *regexp.Regexp
	Callback func(pid uint32)
}

// GetProcessMonitor create a monitor (only once) that register to netlink process events.
//
// This monitor can monitor.Subscribe(callback, filter) callback on particual event
// like process EXEC, EXIT. The callback will be called when the filter will match.
// Filter can be applied on :
//   process name (NAME)
//   by default ANY is applied
//
// Typical initialization:
//   mon := GetProcessMonitor()
//   mon.Subscribe(callback)
//   mon.Initialize()
//
// note: o GetProcessMonitor() will always return the same instance
//         as we can only register once with netlink process event
//       o mon.Subscribe() will subscribe callback before or after the Initialization
//       o mon.Initialize() will scan current processes and call subscribed callback
//
//       o callback{Event: EXIT, Metadata: ANY}   callback is called for all exit events, system wide
//       o callback{Event: EXIT, Metadata: NAME}  callback is called if we seen the process Exec event
//                                                in this case a callback{Event: EXEC, ...} must be registered
func GetProcessMonitor() *ProcessMonitor {
	once.Do(func() {
		processMonitor = &ProcessMonitor{
			isInitialized:      false,
			procEventCallbacks: make(map[ProcessEventType][]*ProcessCallback),
			runningPids:        make(map[uint32]struct{}),
		}
	})

	return processMonitor
}

func (pm *ProcessMonitor) enqueueCallback(callback *ProcessCallback, pid uint32) {
	if callback.Event == EXEC && callback.Metadata != ANY {
		pm.runningPids[pid] = struct{}{}
	}
	pm.callbackRunner <- func() { callback.Callback(pid) }
}

// evalEXECCallback is a best effort and would not return errors, but report them
func (p *ProcessMonitor) evalEXECCallback(c *ProcessCallback, pid uint32) {
	if c.Metadata == ANY {
		p.enqueueCallback(c, pid)
		return
	}

	var err error
	var proc *process.Process
	end := time.Now().Add(10 * time.Millisecond)
	for end.After(time.Now()) {
		// /proc could be slow to update
		proc, err = process.NewProcess(int32(pid))
		if err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
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
	}
}

// Initialize will scan all running processes and execute matching callbacks
// Once it's done all new events from netlink socket will be processed by the main async loop
func (pm *ProcessMonitor) Initialize() error {
	pm.m.Lock()
	defer pm.m.Unlock()

	if pm.isInitialized {
		return fmt.Errorf("Process monitor already initialized")
	}

	pm.events = make(chan netlink.ProcEvent, processMonitorMaxEvents)
	pm.done = make(chan struct{})
	pm.errors = make(chan error)

	if err := netlink.ProcEventMonitor(pm.events, pm.done, pm.errors); err != nil {
		return fmt.Errorf("could not create process monitor: %s", err)
	}

	pm.callbackRunnerDone = make(chan struct{}, runtime.NumCPU())
	pm.callbackRunner = make(chan func(), runtime.NumCPU())
	for i := 0; i < runtime.NumCPU(); i++ {
		pm.wg.Add(1)
		go func() {
			defer pm.wg.Done()
			for {
				select {
				case <-pm.callbackRunnerDone:
					return
				case call, ok := <-pm.callbackRunner:
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
	pm.wg.Add(1)
	go func() {
		defer func() {
			log.Info("netlink process monitor ended")
			pm.wg.Done()
		}()
		for {
			select {
			case <-pm.done:
				return

			case event, ok := <-pm.events:
				if !ok {
					return
				}
				pm.m.Lock()
				if !pm.isInitialized {
					pm.m.Unlock()
					continue
				}

				switch ev := event.Msg.(type) {
				case *netlink.ExecProcEvent:
					for _, c := range pm.procEventCallbacks[EXEC] {
						pm.evalEXECCallback(c, ev.ProcessPid)
					}
				case *netlink.ExitProcEvent:
					for _, c := range pm.procEventCallbacks[EXIT] {
						// Call only the Exit callback if we seen the process pid Exec event
						// if the metadata is ANY we call the callback for all exit events
						if c.Metadata != ANY {
							if _, found := pm.runningPids[ev.ProcessPid]; !found {
								continue
							}
						}
						pm.enqueueCallback(c, ev.ProcessPid)
					}
					delete(pm.runningPids, ev.ProcessPid)
				}
				pm.m.Unlock()

			case err, ok := <-pm.errors:
				if !ok {
					return
				}
				log.Errorf("process montior error: %s", err)
				pm.Stop()
				return
			}
		}
	}()

	fn := func(pid int) error {
		for _, c := range pm.procEventCallbacks[EXEC] {
			pm.evalEXECCallback(c, uint32(pid))
		}
		return nil
	}

	if err := util.WithAllProcs(util.HostProc(), fn); err != nil {
		return fmt.Errorf("process monitor init, scanning all process failed %s", err)
	}
	// enable events to be processed
	pm.isInitialized = true
	return nil
}

// Subscribe register a callback and store it pm.procEventCallbacks[callback.Event] list
// this list is maintained out of order, and the return UnSubscribe function callback
// will remove the previously registered callback from the list
// By design, a callback object can be registered only once
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

		// we are scanning all callbacks remove the one we registered
		// and remove it from the pm.procEventCallbacks[callback.Event] list
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

	pm.m.Lock()
	pm.isInitialized = false
	pm.m.Unlock()
}
