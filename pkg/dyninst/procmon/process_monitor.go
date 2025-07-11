// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package procmon implements a process monitor that can be used to track
// processes and their executables and report interesting processes to the
// actuator.
package procmon

import (
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/container"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Handler is the recipient of processes updates.
type Handler interface {
	HandleUpdate(update ProcessesUpdate)
}

// ProcessesUpdate is a set of updates that the process monitor will send to the
// handler.
type ProcessesUpdate struct {
	Processes []ProcessUpdate
	Removals  []ProcessID
}

// ProcessUpdate is an update to a process's instrumentation configuration.
type ProcessUpdate struct {
	ProcessID  ProcessID
	Executable Executable
	Service    string
	GitInfo    GitInfo
	Container  ContainerInfo
}

// ContainerInfo is information about the container the process is running in.
type ContainerInfo struct {
	// EntityID is the entity id of the process. It is either derived from the
	// container id or inode of the cgroup root.
	EntityID string
	// ContainerID is the container id of the process.
	ContainerID string
}

// GitInfo is information about the git repository and commit sha of the process.
type GitInfo struct {
	// CommitSha is the git commit sha of the process.
	CommitSha string
	// RepositoryURL is the git repository url of the process.
	RepositoryURL string
}

// ProcessMonitor encapsulates the logic of processing events from an event
// monitor and translating them into actuator.ProcessesUpdate calls to the
// actuator.
type ProcessMonitor struct {
	handler            Handler
	procfsRoot         string
	resolver           ContainerResolver
	executableAnalyzer executableAnalyzer

	eventsCh chan event
	doneCh   chan struct{}

	wg           sync.WaitGroup
	shutdownOnce sync.Once
}

// NewProcessMonitor creates a new ProcessMonitor that will send updates to the
// given Actuator.
func NewProcessMonitor(h Handler) *ProcessMonitor {
	return newProcessMonitor(h, kernel.ProcFSRoot(), container.New())
}

// NotifyExec is a callback to notify the monitor that a process has started.
func (pm *ProcessMonitor) NotifyExec(pid uint32) {
	pm.sendEvent(&processEvent{kind: processEventKindExec, pid: pid})
}

// NotifyExit is a callback to notify the monitor that a process has exited.
func (pm *ProcessMonitor) NotifyExit(pid uint32) {
	pm.sendEvent(&processEvent{kind: processEventKindExit, pid: pid})
}

// cacheSize is the size of the cache for the executable analyzer.
//
// The number is somewhat arbitrary, but the logic is something like:
//   - We want to avoid redoing work when processes are created and destroyed
//     frequently.
//   - We don't want to use too much memory. Maybe we're cool with allocating
//     <100K bytes of memory for the caches.
//
// The lru entries aren't super optimized, so they are 6 words per entry. Plus
// some constant overheads and some map overheads due to load factor.
//
// FileKey: 32 bytes (16 for FileHandle + 16 for LastModified)
// HashCacheEntry: 32 bytes (16 for the string header + 16 for the string data)
//
// Per entry: 2*32 bytes (Key) + 48 bytes (lru entry) + 8 bytes (bool) + 8 bytes
// map value.
//
// Each cache entry in each cache works out to roughly ~128 bytes, and we have 2
// caches, so that's ~256 bytes per entry here.
//
// By this math at 64 entries we're looking at <32KiB bytes of memory (16KiB
// for the for the variable data plus some scattered overheads).
const cacheSize = 64

// newProcessMonitor is injectable with a fake FS for tests.
func newProcessMonitor(h Handler, procFS string, resolver ContainerResolver) *ProcessMonitor {
	pm := &ProcessMonitor{
		handler:            h,
		procfsRoot:         procFS,
		resolver:           resolver,
		eventsCh:           make(chan event, 128),
		doneCh:             make(chan struct{}),
		executableAnalyzer: makeExecutableAnalyzer(cacheSize),
	}

	pm.wg.Add(1)
	go func() {
		defer pm.wg.Done()
		run(pm.eventsCh, pm.doneCh, pm)
	}()

	return pm
}

// sendEvent attempts to send an event to the state machine unless we're
// already shutting down.
func (pm *ProcessMonitor) sendEvent(ev event) {
	select {
	case <-pm.doneCh:
	default:
		select {
		case pm.eventsCh <- ev:
		case <-pm.doneCh:
		}
	}
}

// Close requests an orderly shutdown and waits for completion.
func (pm *ProcessMonitor) Close() {
	pm.shutdownOnce.Do(func() {
		log.Debugf("closing process monitor")
		defer log.Debugf("process monitor closed")
		close(pm.doneCh)
		pm.wg.Wait()
	})
}

// analysisFailureLogLimiter is used to limit the rate of logging analysis
// failures.
//
// It is set to infinite in tests.
var analysisFailureLogLimiter = rate.NewLimiter(rate.Every(1*time.Second), 10)

// analyzeProcess analyzes the process with the given PID and sends the result
// to the state machine.
func (pm *ProcessMonitor) analyzeProcess(pid uint32) {
	pm.wg.Add(1)
	go func() {
		defer pm.wg.Done()
		pa, err := analyzeProcess(
			pid, pm.procfsRoot, pm.resolver, &pm.executableAnalyzer,
		)
		shouldLog := err != nil &&
			!os.IsNotExist(err) &&
			analysisFailureLogLimiter.Allow()
		if shouldLog {
			log.Infof("failed to analyze process %d: %v", pid, err)
		}
		pm.sendEvent(&analysisResult{
			pid:             pid,
			err:             err,
			processAnalysis: pa,
		})
	}()
}

func (pm *ProcessMonitor) reportProcessesUpdate(u ProcessesUpdate) {
	pm.handler.HandleUpdate(u)
}

// Ensure ProcessMonitor implements smEffects.
var _ effects = (*ProcessMonitor)(nil)

func run(eventsCh <-chan event, doneCh <-chan struct{}, eff effects) {
	state := newState()
	for {
		select {
		case ev := <-eventsCh:
			state.handle(ev, eff)
		case <-doneCh:
			return
		}
	}
}
