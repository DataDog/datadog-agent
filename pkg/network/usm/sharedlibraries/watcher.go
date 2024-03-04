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

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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

// Rule is a rule to match against a shared library path
type Rule struct {
	Re           *regexp.Regexp
	RegisterCB   func(utils.FilePath) error
	UnregisterCB func(utils.FilePath) error
}

// Watcher provides a way to tie callback functions to the lifecycle of shared libraries
type Watcher struct {
	wg             sync.WaitGroup
	done           chan struct{}
	procRoot       string
	rules          []Rule
	loadEvents     *ddebpf.PerfHandler
	processMonitor *monitor.ProcessMonitor
	registry       *utils.FileRegistry
	ebpfProgram    *ebpfProgram

	// telemetry
	libHits    *telemetry.Counter
	libMatches *telemetry.Counter
}

// NewWatcher creates a new Watcher instance
func NewWatcher(cfg *config.Config, rules ...Rule) (*Watcher, error) {
	ebpfProgram := newEBPFProgram(cfg)
	err := ebpfProgram.Init()
	if err != nil {
		return nil, fmt.Errorf("error initializing shared library program: %w", err)
	}

	return &Watcher{
		wg:             sync.WaitGroup{},
		done:           make(chan struct{}),
		procRoot:       kernel.ProcFSRoot(),
		rules:          rules,
		loadEvents:     ebpfProgram.GetPerfHandler(),
		processMonitor: monitor.GetProcessMonitor(),
		ebpfProgram:    ebpfProgram,
		registry:       utils.NewFileRegistry("shared_libraries"),

		libHits:    telemetry.NewCounter("usm.so_watcher.hits", telemetry.OptPrometheus),
		libMatches: telemetry.NewCounter("usm.so_watcher.matches", telemetry.OptPrometheus),
	}, nil
}

// Stop the Watcher
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

	thisPID, err := kernel.RootNSPID()
	if err != nil {
		log.Warnf("Watcher Start can't get root namespace pid %s", err)
	}

	_ = kernel.WithAllProcs(w.procRoot, func(pid int) error {
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
			// Iterate over the rule, and look for a match.
			for _, r := range w.rules {
				if r.Re.MatchString(path) {
					w.registry.Register(path, uint32(pid), r.RegisterCB, r.UnregisterCB)
					break
				}
			}
		}
		scanner := bufio.NewScanner(bufio.NewReader(maps))
		parseMapsFile(scanner, parseMapsFileCallback)
		return nil
	})

	cleanupExit := w.processMonitor.SubscribeExit(w.registry.Unregister)

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
			w.registry.Clear()
			// marking we're finished.
			w.wg.Done()
		}()

		for {
			select {
			case <-w.done:
				return
			case <-processSync.C:
				processSet := w.registry.GetRegisteredProcesses()
				deletedPids := monitor.FindDeletedProcesses(processSet)
				for deletedPid := range deletedPids {
					w.registry.Unregister(deletedPid)
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

				w.libHits.Add(1)
				path := toBytes(&lib)
				for _, r := range w.rules {
					if r.Re.Match(path) {
						w.libMatches.Add(1)
						w.registry.Register(string(path), lib.Pid, r.RegisterCB, r.UnregisterCB)
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
