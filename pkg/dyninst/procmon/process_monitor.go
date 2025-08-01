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

	mu struct {
		sync.Mutex
		state       state
		isClosed    bool
		analyzeChan chan<- uint32
	}

	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewProcessMonitor creates a new ProcessMonitor that will send updates to the
// given Actuator.
func NewProcessMonitor(h Handler) *ProcessMonitor {
	return newProcessMonitor(h, kernel.ProcFSRoot(), container.New())
}

// NotifyExec is a callback to notify the monitor that a process has started.
func (pm *ProcessMonitor) NotifyExec(pid uint32) {
	handleEvent(pm, (*state).handleProcessEvent, processEvent{
		kind: processEventKindExec, pid: pid,
	})
}

// NotifyExit is a callback to notify the monitor that a process has exited.
func (pm *ProcessMonitor) NotifyExit(pid uint32) {
	handleEvent(pm, (*state).handleProcessEvent, processEvent{
		kind: processEventKindExit, pid: pid,
	})
}

// cacheSize is the size of the cache for the executable analyzer.
//
// This value balances performance and memory usage:
//   - Avoids redundant analysis when processes are created/destroyed frequently
//   - Limits memory usage to approximately 32KiB for both caches combined
//
// Memory calculation (approximate):
//
//	FileKey: 32 bytes (16 for FileHandle + 16 for LastModified)
//	HashCacheEntry: 32 bytes (16 for string header + 16 for string data)
//	LRU overhead: ~48 bytes per entry
//	Map overhead: ~56 bytes per entry (~32 bytes for the key, ~8 bytes for
//	 			  the value, and conservatively ~16 bytes for the map
//	 			  overhead and load factor)
//
//	Total per entry: ~144 bytes (32 + 8 + 48 + 56)
//	At 64 entries: ~9KiB from entries + constant overheads < 16KiB per cache
const cacheSize = 64

// newProcessMonitor is injectable with a fake FS for tests.
func newProcessMonitor(
	h Handler, procFS string, resolver ContainerResolver,
) *ProcessMonitor {
	analyzeChan := make(chan uint32, 1) // this will never block
	pm := &ProcessMonitor{
		handler:            h,
		procfsRoot:         procFS,
		resolver:           resolver,
		executableAnalyzer: makeExecutableAnalyzer(cacheSize),
	}
	pm.mu.state = makeState()
	pm.mu.analyzeChan = analyzeChan

	// Run an analysis worker goroutine.
	pm.wg.Add(1)
	go func() {
		defer pm.wg.Done()
		for {
			pid, ok := <-analyzeChan
			if !ok {
				return
			}
			pm.analyzeProcess(pid)
		}
	}()

	return pm
}

func handleEvent[Ev any](pm *ProcessMonitor, f func(*state, Ev), ev Ev) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.mu.isClosed {
		return
	}
	f(&pm.mu.state, ev)
	pm.mu.state.analyzeOrReport((*lockedProcessMonitor)(pm))
}

// Close requests an orderly shutdown and waits for completion.
func (pm *ProcessMonitor) Close() {
	pm.closeOnce.Do(func() {
		log.Debugf("closing process monitor")
		defer log.Debugf("process monitor closed")
		defer pm.wg.Wait()
		pm.mu.Lock()
		defer pm.mu.Unlock()
		close(pm.mu.analyzeChan)
		pm.mu.isClosed = true
	})
}

// analysisFailureLogLimiter is used to limit the rate of logging analysis
// failures.
//
// It is set to infinite in tests.
var analysisFailureLogLimiter = rate.NewLimiter(rate.Every(1*time.Second), 10)

// Limit the rate of logging permission errors, because if we see them, we'll
// probably see a lot of them.
var analysisFailurePermissionLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

// analyzeProcess analyzes the process with the given PID and sends the result
// to the state machine.
func (pm *ProcessMonitor) analyzeProcess(pid uint32) {
	pa, err := analyzeProcess(
		pid, pm.procfsRoot, pm.resolver, pm.executableAnalyzer,
	)
	shouldLog := err != nil && analysisFailureLogLimiter.Allow()
	if shouldLog {
		pid := pid
		if os.IsPermission(err) && !analysisFailurePermissionLogLimiter.Allow() {
			// We don't want to be too noisy about permission errors, but we
			// do want to learn about them as they are a sign of a problem.
			log.Debugf("failed to analyze process %d: %v", pid, err)
		} else {
			log.Infof("failed to analyze process %d: %v", pid, err)
		}
	}
	handleEvent(pm, (*state).handleAnalysisResult, analysisResult{
		pid:             pid,
		err:             err,
		processAnalysis: pa,
	})
}

type lockedProcessMonitor ProcessMonitor

func (pm *lockedProcessMonitor) analyzeProcess(pid uint32) {
	select {
	case pm.mu.analyzeChan <- pid:
	default:
		// This should never happen, but if it does, we'll log it and shutdown
		// the monitor rather than potentially crashing the process.
		log.Errorf(
			"invariant violation: process monitor would block, shutting down",
		)
		go (*ProcessMonitor)(pm).Close() // must not be holding the mutex
	}
}

func (pm *lockedProcessMonitor) reportProcessesUpdate(u ProcessesUpdate) {
	pm.handler.HandleUpdate(u)
}

// Ensure ProcessMonitor implements smEffects.
var _ effects = (*lockedProcessMonitor)(nil)
