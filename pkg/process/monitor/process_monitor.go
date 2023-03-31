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

	"github.com/vishvananda/netlink"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process"
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
// Initialize() will scan the current process and will call the subscribed callbacks.
//
// callbacks will be executed in parallel via a pool of goroutines (runtime.NumCPU())
// callbackRunner is callbacks queue. The queue size is set by processMonitorMaxEvents
//
// Multiple team can use the same ProcessMonitor,
// the callers need to guarantee calling each Initialize() Stop() one single time
// this maintains an internal reference counter
//
// ProcessMonitor require root or CAP_NET_ADMIN capabilities
type ProcessMonitor struct {
	m        sync.Mutex
	wg       sync.WaitGroup
	refcount *atomic.Int32

	isInitialized *atomic.Bool

	// chan push done by vishvananda/netlink library
	events chan netlink.ProcEvent
	done   chan struct{}
	errors chan error

	// callback registration and parallel execution management
	procEventCallbacks map[ProcessEventType][]*ProcessCallback
	runningPids        map[uint32]interface{}
	callbackRunner     chan func()

	// monitor stats
	eventCount int
	execCount  int
	exitCount  int
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

type metadataName struct {
	Name string
}

type ProcessCallback struct {
	Event    ProcessEventType
	Metadata ProcessMetadataField
	Regex    *regexp.Regexp
	Callback func(pid uint32)
}

// GetProcessMonitor create a monitor (only once) that register to netlink process events.
//
// This monitor can monitor.Subscribe(callback, filter) callback on particular event
// like process EXEC, EXIT. The callback will be called when the filter will match.
// Filter can be applied on :
//
//	process name (NAME)
//	by default ANY is applied
//
// Typical initialization:
//
//	mon := GetProcessMonitor()
//	mon.Subscribe(callback)
//	mon.Initialize()
//
// note: o GetProcessMonitor() will always return the same instance
//
//	  as we can only register once with netlink process event
//	o mon.Subscribe() will subscribe callback before or after the Initialization
//	o mon.Initialize() will scan current processes and call subscribed callback
//
//	o callback{Event: EXIT, Metadata: ANY}   callback is called for all exit events (system-wide)
//	o callback{Event: EXIT, Metadata: NAME}  callback will be called if we have seen the process Exec event,
//	                                         the metadata will be saved between Exec and Exit event per pid
//	                                         then the Exit callback will evaluate the same metadata on Exit.
//	                                         We need to save the metadata here as /proc/pid doesn't exist anymore.
func GetProcessMonitor() *ProcessMonitor {
	once.Do(func() {
		processMonitor = &ProcessMonitor{
			procEventCallbacks: make(map[ProcessEventType][]*ProcessCallback),
			runningPids:        make(map[uint32]interface{}),
		}
		processMonitor.refcount = atomic.NewInt32(0)
		processMonitor.isInitialized = atomic.NewBool(false)
	})

	return processMonitor
}

func (pm *ProcessMonitor) enqueueCallback(callback *ProcessCallback, pid uint32, metadata interface{}) {
	if callback.Event == EXEC && callback.Metadata != ANY {
		switch callback.Metadata {
		case NAME:
			pm.runningPids[pid] = metadata
		}
	}
	pm.callbackRunner <- func() { callback.Callback(pid) }
}

// evalEXECCallback is a best effort and would not return errors, but report them
func (pm *ProcessMonitor) evalEXECCallback(c *ProcessCallback, pid uint32) {
	if c.Metadata == ANY {
		pm.enqueueCallback(c, pid, nil)
		return
	}

	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		// We receive the Exec event first and /proc could be slow to update
		end := time.Now().Add(10 * time.Millisecond)
		for end.After(time.Now()) {
			proc, err = process.NewProcess(int32(pid))
			if err == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	if err != nil {
		// short living process can hit here (or later proc.Name() parsing)
		// as they already exited when we try to find them in /proc
		// so let's be quiet on the logs as there not much to do here
		return
	}

	switch c.Metadata {
	case NAME:
		pname, err := proc.Name()
		if err != nil {
			log.Debugf("process %d name parsing failed %s", pid, err)
			return
		}
		if c.Regex.MatchString(pname) {
			pm.enqueueCallback(c, pid, metadataName{Name: pname})
		}
	}
}

// evalEXITCallback will evaluate the metadata saved by the Exec callback and the callback accordingly
// please refer to GetProcessMonitor documentation
func (pm *ProcessMonitor) evalEXITCallback(c *ProcessCallback, pid uint32) {
	switch c.Metadata {
	case NAME:
		metadata, found := pm.runningPids[pid]
		if !found {
			// we can hit here if a process started before the Exec callback has been registered
			// and the process Exit, so we don't find his metadata
			return
		}
		pname := metadata.(metadataName).Name
		if c.Regex.MatchString(pname) {
			pm.enqueueCallback(c, pid, metadata)
		}
	case ANY:
		pm.enqueueCallback(c, pid, nil)
	}
}

func (pm *ProcessMonitor) closeDone() {
	pm.m.Lock()
	if pm.done != nil {
		close(pm.done)
		pm.done = nil
	}
	pm.m.Unlock()
}

// Initialize will scan all running processes and execute matching callbacks
// Once it's done all new events from netlink socket will be processed by the main async loop
func (pm *ProcessMonitor) Initialize() error {
	pm.m.Lock()
	defer pm.m.Unlock()

	pm.refcount.Inc()
	if pm.isInitialized.Load() {
		return nil
	}

	pm.events = make(chan netlink.ProcEvent, processMonitorMaxEvents)
	pm.done = make(chan struct{})
	pm.errors = make(chan error, 10)

	hostProc := util.HostProc()
	if err := util.WithRootNS(hostProc, func() error {
		return netlink.ProcEventMonitor(pm.events, pm.done, pm.errors)

	}); err != nil {
		return fmt.Errorf("couldn't initialize process monitor: %s", err)
	}

	pm.callbackRunner = make(chan func(), runtime.NumCPU())
	for i := 0; i < runtime.NumCPU(); i++ {
		pm.wg.Add(1)
		go func() {
			defer pm.wg.Done()
			for call := range pm.callbackRunner {
				if call != nil {
					call()
				}
			}
		}()
	}

	// This is the main async loop, where we process "processes" events from netlink socket
	// events are dropped until
	pm.wg.Add(1)
	go func() {
		logTicker := time.NewTicker(2 * time.Minute)

		defer func() {
			logTicker.Stop()
			close(pm.callbackRunner)
			pm.isInitialized.Store(false)
			pm.wg.Done()
			log.Info("netlink process monitor ended")
		}()

		for {
			select {
			case <-pm.done:
				return
			case event, ok := <-pm.events:
				if !ok {
					pm.closeDone()
					return
				}
				if !pm.isInitialized.Load() {
					continue
				}

				pm.m.Lock()
				pm.eventCount += 1

				switch ev := event.Msg.(type) {
				case *netlink.ExecProcEvent:
					for _, c := range pm.procEventCallbacks[EXEC] {
						pm.execCount += 1
						pm.evalEXECCallback(c, ev.ProcessPid)
					}
				case *netlink.ExitProcEvent:
					for _, c := range pm.procEventCallbacks[EXIT] {
						pm.exitCount += 1
						pm.evalEXITCallback(c, ev.ProcessPid)
					}
					delete(pm.runningPids, ev.ProcessPid)
				}
				pm.m.Unlock()

			case err, ok := <-pm.errors:
				if !ok {
					pm.closeDone()
					return
				}
				log.Errorf("process monitor error: %s", err)
				// closing netlink subscription
				pm.closeDone()
				return

			case <-logTicker.C:
				pm.logStats()
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
	pm.isInitialized.Store(true)
	return nil
}

// Subscribe register a callback and store it pm.procEventCallbacks[callback.Event] list
// this list is maintained out of order, and the return UnSubscribe function callback
// will remove the previously registered callback from the list
//
// By design : 1/ a callback object can be registered only once
//
//	2/ Exec callback with a Metadata (!=ANY) must be registered before the sibling Exit metadata,
//	   otherwise the Subscribe() will return an error as no metadata will be saved between Exec and Exit,
//	   please refer to GetProcessMonitor()
func (pm *ProcessMonitor) Subscribe(callback *ProcessCallback) (UnSubscribe func(), err error) {
	pm.m.Lock()
	defer pm.m.Unlock()

	for _, c := range pm.procEventCallbacks[callback.Event] {
		if c == callback {
			return nil, errors.New("same callback can't be registered twice")
		}
	}

	// check if the sibling Exec callback exist
	if callback.Event == EXIT && callback.Metadata != ANY {
		foundSibling := false
		for _, c := range pm.procEventCallbacks[EXEC] {
			if c.Metadata == callback.Metadata && c.Regex.String() == callback.Regex.String() {
				foundSibling = true
				break
			}
		}
		if !foundSibling {
			return nil, errors.New("no Exec callback has been found with the same Metadata and Regex, please Subscribe(Exec callback, Metadata) first")
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
	if pm.refcount.Load() == 0 {
		return
	}
	if pm.refcount.Dec() > 0 {
		return
	}
	pm.closeDone()
	pm.wg.Wait()
}

func (pm *ProcessMonitor) logStats() {
	log.Debugf("process monitor stats - total events: %v; exec events: %v; exit events: %v", pm.eventCount, pm.execCount, pm.exitCount)

	pm.eventCount = 0
	pm.execCount = 0
	pm.exitCount = 0
}
