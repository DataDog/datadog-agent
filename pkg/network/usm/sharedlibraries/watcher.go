// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package sharedlibraries

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unsafe"

	"go.uber.org/atomic"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// The interval of the periodic scan for terminated processes. Increasing the interval, might cause larger spikes in cpu
	// and lowering it might cause constant cpu usage.
	scanTerminatedProcessesInterval = 30 * time.Second
)

func toLibPath(data []byte) libPath {
	return *(*libPath)(unsafe.Pointer(&data[0]))
}

func toBytes(l *libPath) []byte {
	return l.Buf[:l.Len]
}

type Rule struct {
	Re           *regexp.Regexp
	RegisterCB   func(id PathIdentifier, root string, path string) error
	UnregisterCB func(id PathIdentifier) error
}

// Watcher provides a way to tie callback functions to the lifecycle of shared libraries
type Watcher struct {
	wg             sync.WaitGroup
	done           chan struct{}
	procRoot       string
	rules          []Rule
	loadEvents     *ddebpf.PerfHandler
	processMonitor *monitor.ProcessMonitor
	registry       *soRegistry
	ebpfProgram    *ebpfProgram
}

func NewWatcher(cfg *config.Config, bpfTelemetry *errtelemetry.EBPFTelemetry, rules ...Rule) (*Watcher, error) {
	ebpfProgram := newEBPFProgram(cfg, bpfTelemetry)
	err := ebpfProgram.Init()
	if err != nil {
		return nil, fmt.Errorf("error initializing shared library program: %w", err)
	}

	metricGroup := telemetry.NewMetricGroup(
		"usm.so_watcher",
		telemetry.OptPayloadTelemetry,
	)
	return &Watcher{
		wg:             sync.WaitGroup{},
		done:           make(chan struct{}),
		procRoot:       util.GetProcRoot(),
		rules:          rules,
		loadEvents:     ebpfProgram.GetPerfHandler(),
		processMonitor: monitor.GetProcessMonitor(),
		ebpfProgram:    ebpfProgram,
		registry: &soRegistry{
			byID:          make(map[PathIdentifier]*soRegistration),
			byPID:         make(map[uint32]pathIdentifierSet),
			blocklistByID: make(pathIdentifierSet),

			telemetry: soRegistryTelemetry{
				libHookFailed:               metricGroup.NewCounter("hook_failed"),
				libRegistered:               metricGroup.NewCounter("registered"),
				libAlreadyRegistered:        metricGroup.NewCounter("already_registered"),
				libBlocked:                  metricGroup.NewCounter("blocked"),
				libUnregistered:             metricGroup.NewCounter("unregistered"),
				libUnregisterNoCB:           metricGroup.NewCounter("unregister_no_callback"),
				libUnregisterErrors:         metricGroup.NewCounter("unregister_errors"),
				libUnregisterFailedCB:       metricGroup.NewCounter("unregister_failed_cb"),
				libUnregisterPathIDNotFound: metricGroup.NewCounter("unregister_path_id_not_found"),
				libHits:                     metricGroup.NewCounter("hits"),
				libMatches:                  metricGroup.NewCounter("matches"),
			},
		},
	}, nil
}

type soRegistryTelemetry struct {
	// a library can be :
	//  o Registered : it's a new library
	//  o AlreadyRegistered : we have already hooked (uprobe) this library (unique by pathID)
	//  o HookFailed : uprobe registration failed for one library
	//  o Blocked : previous uprobe registration failed, so we block further call
	//  o Unregistered : a library hook is unregistered, meaning there are no more refcount to the corresponding pathID
	//  o UnregisterNoCB : unregister event has been done but the rule doesn't have an unregister callback
	//  o UnregisterErrors : we encounter an error during the unregistration, looks at the logs for further details
	//  o UnregisterFailedCB : we encounter an error during the callback unregistration, looks at the logs for further details
	//  o UnregisterPathIDNotFound : we can't find the pathID registration, it's a bug, this value should be always 0
	libRegistered               *telemetry.Counter
	libAlreadyRegistered        *telemetry.Counter
	libHookFailed               *telemetry.Counter
	libBlocked                  *telemetry.Counter
	libUnregistered             *telemetry.Counter
	libUnregisterNoCB           *telemetry.Counter
	libUnregisterErrors         *telemetry.Counter
	libUnregisterFailedCB       *telemetry.Counter
	libUnregisterPathIDNotFound *telemetry.Counter

	// numbers of library events from the kernel filter (Hits) and matching (Matches) the registered rules
	libHits    *telemetry.Counter
	libMatches *telemetry.Counter
}

type soRegistration struct {
	uniqueProcessesCount atomic.Int32
	unregisterCB         func(PathIdentifier) error

	// we are sharing the telemetry from soRegistry
	telemetry *soRegistryTelemetry
}

// unregister return true if there are no more reference to this registration
func (r *soRegistration) unregisterPath(pathID PathIdentifier) bool {
	currentUniqueProcessesCount := r.uniqueProcessesCount.Dec()
	if currentUniqueProcessesCount > 0 {
		return false
	}
	if currentUniqueProcessesCount < 0 {
		log.Errorf("unregistered %+v too much (current counter %v)", pathID, currentUniqueProcessesCount)
		r.telemetry.libUnregisterErrors.Add(1)
		return true
	}

	// currentUniqueProcessesCount is 0, thus we should unregister.
	if r.unregisterCB != nil {
		if err := r.unregisterCB(pathID); err != nil {
			// Even if we fail here, we have to return true, as best effort methodology.
			// We cannot handle the failure, and thus we should continue.
			log.Errorf("error while unregistering %s : %s", pathID.String(), err)
			r.telemetry.libUnregisterFailedCB.Add(1)
		}
	} else {
		r.telemetry.libUnregisterNoCB.Add(1)
	}
	r.telemetry.libUnregistered.Add(1)
	return true
}

func (w *Watcher) Stop() {
	if w == nil {
		return
	}

	w.ebpfProgram.Stop()
	close(w.done)
	w.wg.Wait()
}

type parseMapsFileCB func(path string)

// parseMapsFile takes in a bufio.Scanner representing a memory mapping of /proc/<PID>/maps file, and a callback to be
// applied on the paths extracted from the file. We're extracting only actual paths on the file system, and ignoring
// anonymous memory regions.
//
// Example for entries in the `maps` file:
// 7f135146b000-7f135147a000 r--p 00000000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
// 7f135147a000-7f1351521000 r-xp 0000f000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
// 7f1351521000-7f13515b8000 r--p 000b6000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
// 7f13515b8000-7f13515b9000 r--p 0014c000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
func parseMapsFile(scanner *bufio.Scanner, callback parseMapsFileCB) {
	// The maps file can have multiple entries of the same loaded file, the cache is meant to ensure, we're not wasting
	// time and memory on "duplicated" hooking.
	cache := make(map[string]struct{})
	for scanner.Scan() {
		line := scanner.Text()
		cols := strings.Fields(line)
		// ensuring we have exactly 6 elements (skip '(deleted)' entries) in the line, and the 4th element (inode) is
		// not zero (indicates it is a path, and not an anonymous path).
		if len(cols) == 6 && cols[4] != "0" {
			// Check if we've seen the same path before, if so, continue to the next.
			if _, exists := cache[cols[5]]; exists {
				continue
			}
			// We didn't process the path, so cache it to avoid future re-processing.
			cache[cols[5]] = struct{}{}

			// Apply the given callback on the path.
			callback(cols[5])
		}
	}
}

// Start consuming shared-library events
func (w *Watcher) Start() {
	if w == nil {
		return
	}

	thisPID, err := util.GetRootNSPID()
	if err != nil {
		log.Warnf("Watcher Start can't get root namespace pid %s", err)
	}

	_ = util.WithAllProcs(w.procRoot, func(pid int) error {
		if pid == thisPID { // don't scan ourself
			return nil
		}

		mapsPath := fmt.Sprintf("%s/%d/maps", w.procRoot, pid)
		maps, err := os.Open(mapsPath)
		if err != nil {
			log.Debugf("process %d parsing failed %s", pid, err)
			return nil
		}
		defer maps.Close()

		// Creating a callback to be applied on the paths extracted from the `maps` file.
		// We're creating the callback here, as we need the pid (which varies between iterations).
		parseMapsFileCallback := func(path string) {
			root := fmt.Sprintf("%s/%d/root", w.procRoot, pid)
			// Iterate over the rule, and look for a match.
			for _, r := range w.rules {
				if r.Re.MatchString(path) {
					w.registry.register(root, path, uint32(pid), r)
					break
				}
			}
		}
		scanner := bufio.NewScanner(bufio.NewReader(maps))
		parseMapsFile(scanner, parseMapsFileCallback)
		return nil
	})

	cleanupExit := w.processMonitor.SubscribeExit(w.registry.unregister)

	w.wg.Add(1)
	go func() {
		processSync := time.NewTicker(scanTerminatedProcessesInterval)

		defer func() {
			processSync.Stop()
			// Removing the registration of our hook.
			cleanupExit()
			// Stopping the process monitor (if we're the last instance)
			w.processMonitor.Stop()
			// Cleaning up all active hooks.
			w.registry.cleanup()
			// marking we're finished.
			w.wg.Done()
		}()

		for {
			select {
			case <-w.done:
				return
			case <-processSync.C:
				processSet := make(map[int32]struct{})
				w.registry.m.RLock()
				for pid := range w.registry.byPID {
					processSet[int32(pid)] = struct{}{}
				}
				w.registry.m.RUnlock()

				deletedPids := monitor.FindDeletedProcesses(processSet)
				for deletedPid := range deletedPids {
					w.registry.unregister(int(deletedPid))
				}
			case event, ok := <-w.loadEvents.DataChannel:
				if !ok {
					return
				}

				lib := toLibPath(event.Data)
				if int(lib.Pid) == thisPID {
					// don't scan ourself
					event.Done()
					continue
				}

				w.registry.telemetry.libHits.Add(1)
				path := toBytes(&lib)
				libPath := string(path)
				procPid := fmt.Sprintf("%s/%d", w.procRoot, lib.Pid)
				root := procPid + "/root"
				// use cwd of the process as root if the path is relative
				if libPath[0] != '/' {
					root = procPid + "/cwd"
					libPath = "/" + libPath
				}

				for _, r := range w.rules {
					if r.Re.Match(path) {
						w.registry.telemetry.libMatches.Add(1)
						w.registry.register(root, libPath, lib.Pid, r)
						break
					}
				}
				event.Done()
			case <-w.loadEvents.LostChannel:
				// Nothing to do in this case
				break
			}
		}
	}()

	err = w.ebpfProgram.Start()
	if err != nil {
		log.Errorf("error starting shared library detection eBPF program: %s", err)
	}
}
