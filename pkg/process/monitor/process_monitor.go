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
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vishvananda/netlink"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	processMonitorMaxEvents   = 2048
	eventLoopBackMaxIdleTasks = 1000
)

var (
	initOnce       sync.Once
	processMonitor = &ProcessMonitor{
		processExecCallbacks: sync.Map{},
		processExitCallbacks: sync.Map{},
	}
)

// ProcessMonitor uses netlink process events like Exec and Exit and activate the registered callbacks for the relevant
// events.
// ProcessMonitor require root or CAP_NET_ADMIN capabilities
type ProcessMonitor struct {
	wg       sync.WaitGroup
	refcount atomic.Int32
	done     chan struct{}

	// netlink channels for process event monitor.
	netlinkEventsChannel chan netlink.ProcEvent
	netlinkDoneChannel   chan struct{}
	netlinkErrorsChannel chan error

	// callback registration and parallel execution management
	processExecCallbacks sync.Map
	processExitCallbacks sync.Map
	runningPids          sync.Map
	callbackRunner       chan func()

	// monitor stats
	eventCount atomic.Uint32
	execCount  atomic.Uint32
	exitCount  atomic.Uint32
}

type ProcessFilterType int

const (
	ANY ProcessFilterType = iota
	NAME
)

type ProcessCallback struct {
	FilterType ProcessFilterType
	Regex      *regexp.Regexp
	Callback   func(pid int)
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
//
// thread safe.
func GetProcessMonitor() *ProcessMonitor {
	return processMonitor
}

// handleProcessExec is a callback function called on a given pid that represents a new process.
// we're iterating the relevant callbacks and trigger them.
func (pm *ProcessMonitor) handleProcessExec(pid int) {
	pm.processExecCallbacks.Range(func(key, _ any) bool {
		callback := key.(*ProcessCallback)
		pm.handleProcessExecInner(callback, pid)
		return true
	})
}

// handleProcessExecInner ensures we should run the given callback on the given PID, and operates if needed.
// A helper function for handleProcessExec for readability.
func (pm *ProcessMonitor) handleProcessExecInner(c *ProcessCallback, pid int) {
	switch c.FilterType {
	case NAME:
		name, err := readProcessName(pid)
		if err != nil {
			// We failed reading the process name, probably the entry in `<HOST_PROC>/<pid>/comm` is not ready yet.
			for i := 0; i < 5; i++ {
				name, err = readProcessName(pid)
				if err == nil {
					break
				}
				time.Sleep(time.Millisecond)
			}
		}
		if err != nil {
			// short living process can hit here as they already exited when we try to find them in `<HOST_PROC>`.
			log.Debugf("process %d name parsing failed %s", pid, err)
			return
		}
		if !c.Regex.MatchString(name) {
			return
		}
		pm.runningPids.Store(pid, name)
		fallthrough
	case ANY:
		pm.callbackRunner <- func() { c.Callback(pid) }
	}
}

// handleProcessExit is a callback function called on a given pid that represents an exit event.
// we're iterating the relevant callbacks and trigger them.
func (pm *ProcessMonitor) handleProcessExit(pid int) {
	pm.processExitCallbacks.Range(func(key, _ any) bool {
		callback := key.(*ProcessCallback)
		pm.handleProcessExitInner(callback, pid)
		return true
	})

	pm.runningPids.Delete(pid)
}

// handleProcessExitInner ensures we should run the given callback on the given PID, and operates if needed.
// A helper function for handleProcessExit for readability.
func (pm *ProcessMonitor) handleProcessExitInner(c *ProcessCallback, pid int) {
	switch c.FilterType {
	case NAME:
		metadata, found := pm.runningPids.Load(pid)
		if !found {
			// we can hit here if a process started before the Exec callback has been registered
			// and the process Exit, so we don't find his metadata
			return
		}
		pname := metadata.(string)
		if !c.Regex.MatchString(pname) {
			return
		}
		fallthrough
	case ANY:
		pm.callbackRunner <- func() { c.Callback(pid) }
	}
}

// initNetlinkProcessEventMonitor initialize the netlink socket filter for process event monitor.
func (pm *ProcessMonitor) initNetlinkProcessEventMonitor() error {
	pm.netlinkDoneChannel = make(chan struct{})
	pm.netlinkErrorsChannel = make(chan error, 10)
	pm.netlinkEventsChannel = make(chan netlink.ProcEvent, processMonitorMaxEvents)

	if err := util.WithRootNS(util.GetProcRoot(), func() error {
		return netlink.ProcEventMonitor(pm.netlinkEventsChannel, pm.netlinkDoneChannel, pm.netlinkErrorsChannel)
	}); err != nil {
		return fmt.Errorf("couldn't initialize process monitor: %s", err)
	}

	return nil
}

// initCallbackRunner runs multiple workers that run tasks sent over a queue.
func (pm *ProcessMonitor) initCallbackRunner() {
	cpuNum := runtime.NumCPU()
	pm.callbackRunner = make(chan func(), eventLoopBackMaxIdleTasks)
	pm.wg.Add(cpuNum)
	for i := 0; i < cpuNum; i++ {
		go func() {
			defer pm.wg.Done()
			for call := range pm.callbackRunner {
				if call != nil {
					call()
				}
			}
		}()
	}
}

// mainEventLoop is an event loop receiving events from netlink, or periodic events, and handles them.
func (pm *ProcessMonitor) mainEventLoop() {
	logTicker := time.NewTicker(2 * time.Minute)
	processSync := time.NewTicker(time.Second * 30)

	defer func() {
		processSync.Stop()
		logTicker.Stop()
		// Marking netlink to stop, so we won't get any new events.
		close(pm.netlinkDoneChannel)
		// waiting for the callbacks runners to finish
		close(pm.callbackRunner)
		// Cleaning current PIDs.
		pm.runningPids.Range(func(pid, _ any) bool {
			pm.processExitCallbacks.Range(func(cb, _ any) bool {
				callback := cb.(*ProcessCallback)
				callback.Callback(pid.(int))
				return true
			})
			pm.runningPids.Delete(pid)
			return true
		})
		// Before shutting down, making sure we're cleaning all resources.
		pm.wg.Done()
	}()

	for {
		select {
		case <-pm.done:
			return
		case event, ok := <-pm.netlinkEventsChannel:
			if !ok {
				return
			}

			pm.eventCount.Inc()

			switch ev := event.Msg.(type) {
			case *netlink.ExecProcEvent:
				pm.execCount.Inc()
				pm.handleProcessExec(int(ev.ProcessPid))
			case *netlink.ExitProcEvent:
				pm.exitCount.Inc()
				pm.handleProcessExit(int(ev.ProcessPid))
			}
		case err, ok := <-pm.netlinkErrorsChannel:
			if !ok {
				return
			}
			log.Errorf("process monitor error: %s", err)
			log.Info("re-initializing process monitor")
			pm.netlinkDoneChannel <- struct{}{}
			if err := pm.initNetlinkProcessEventMonitor(); err != nil {
				log.Errorf("failed re-initializing process monitor: %s", err)
				return
			}
		case <-logTicker.C:
			log.Debugf("process monitor stats - total events: %d; exec events: %d; exit events: %d",
				pm.eventCount.Swap(0), pm.execCount.Swap(0), pm.exitCount.Swap(0))
		case <-processSync.C:
			processSet := make(map[int32]struct{}, 10)
			pm.runningPids.Range(func(key, _ any) bool {
				processSet[int32(key.(int))] = struct{}{}
				return true
			})
			if len(processSet) == 0 {
				continue
			}
			go func() {
				deletedPids := FindDeletedProcesses(processSet)
				for deletedPid := range deletedPids {
					pm.handleProcessExit(int(deletedPid))
				}
			}()
		}
	}
}

// Initialize setting up the process monitor only once, no matter how many times it was called.
// The initialization order:
//  1. Initializes callback workers.
//  2. Initializes the netlink process monitor.
//  2. Run the main event loop in a goroutine.
//  4. Scans already running processes and call the Exec callbacks on them.
func (pm *ProcessMonitor) Initialize() error {
	processMonitor.refcount.Inc()
	var initErr error
	initOnce.Do(
		func() {
			pm.done = make(chan struct{})
			pm.runningPids = sync.Map{}
			pm.initCallbackRunner()

			pm.wg.Add(1)
			// Setting up the main loop
			pm.netlinkDoneChannel = make(chan struct{})
			pm.netlinkErrorsChannel = make(chan error, 10)
			pm.netlinkEventsChannel = make(chan netlink.ProcEvent, processMonitorMaxEvents)

			go pm.mainEventLoop()

			if err := util.WithRootNS(util.GetProcRoot(), func() error {
				return netlink.ProcEventMonitor(pm.netlinkEventsChannel, pm.netlinkDoneChannel, pm.netlinkErrorsChannel)
			}); err != nil {
				initErr = fmt.Errorf("couldn't initialize process monitor: %s", err)
			}

			handleProcessExecWrapper := func(pid int) error {
				pm.handleProcessExec(pid)
				return nil
			}
			// Scanning already running processes
			if err := util.WithAllProcs(util.GetProcRoot(), handleProcessExecWrapper); err != nil {
				initErr = fmt.Errorf("process monitor init, scanning all process failed %s", err)
				return
			}
		},
	)
	return initErr
}

// SubscribeExec register an exec callback and returns unsubscribe function callback that removes the callback.
//
// A callback can be registered only once, callback with a filter type (not ANY) must be registered before the matching
// Exit callback.
func (pm *ProcessMonitor) SubscribeExec(callback *ProcessCallback) (func(), error) {
	if _, exists := pm.processExecCallbacks.LoadOrStore(callback, struct{}{}); exists {
		return nil, errors.New("same callback can't be registered twice")
	}

	// UnSubscribe()
	return func() {
		pm.processExecCallbacks.Delete(callback)
	}, nil
}

// SubscribeExit register an exit callback and returns unsubscribe function callback that removes the callback.
func (pm *ProcessMonitor) SubscribeExit(callback *ProcessCallback) (func(), error) {
	if _, exists := pm.processExitCallbacks.Load(callback); exists {
		return nil, errors.New("same callback can't be registered twice")
	}

	// check if the sibling Exec callback exist
	if callback.FilterType != ANY {
		foundSibling := false
		pm.processExecCallbacks.Range(func(key, _ any) bool {
			execCallback := key.(*ProcessCallback)
			if callback.FilterType == execCallback.FilterType && callback.Regex.String() == execCallback.Regex.String() {
				foundSibling = true
				// stopping iteration.
				return false
			}
			return true
		})
		if !foundSibling {
			return nil, errors.New("no Exec callback has been found with the same Metadata and Regex, please Subscribe(Exec callback, Metadata) first")
		}
	}

	pm.processExitCallbacks.Store(callback, struct{}{})

	// UnSubscribe()
	return func() {
		pm.processExitCallbacks.Delete(callback)
	}, nil
}

// Stop decreasing the refcount, and if we reach 0 we terminate the main event loop.
func (pm *ProcessMonitor) Stop() {
	if pm.refcount.Dec() != 0 {
		if pm.refcount.Load() < 0 {
			pm.refcount.Swap(0)
		}
		return
	}

	close(pm.done)
	pm.wg.Wait()
	// that's being done for testing purposes.
	// As tests are running altogether, initOne and processMonitor are being created only once per compilation unit
	// thus, the first test works without an issue, but the second test has troubles.
	initOnce = sync.Once{}
	pm.processExecCallbacks = sync.Map{}
	pm.processExitCallbacks = sync.Map{}
}

// FindDeletedProcesses returns the terminated PIDs from the given map.
func FindDeletedProcesses[V any](pids map[int32]V) map[int32]struct{} {
	existingPids := make(map[int32]struct{}, len(pids))

	procIter := func(pid int) error {
		if _, exists := pids[int32(pid)]; exists {
			existingPids[int32(pid)] = struct{}{}
		}
		return nil
	}
	// Scanning already running processes
	if err := util.WithAllProcs(util.GetProcRoot(), procIter); err != nil {
		return nil
	}

	res := make(map[int32]struct{}, len(pids)-len(existingPids))
	for pid := range pids {
		if _, exists := existingPids[pid]; exists {
			continue
		}
		res[pid] = struct{}{}
	}

	return res
}

// readProcessName is a simple method to return the process name for a given PID.
// The method is much faster and efficient that using process.NewProcess(pid).Name().
func readProcessName(pid int) (string, error) {
	content, err := os.ReadFile(filepath.Join(util.GetProcRoot(), strconv.Itoa(pid), "comm"))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(content)), nil
}
