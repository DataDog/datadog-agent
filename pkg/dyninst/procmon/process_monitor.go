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
	"sync"
	"time"

	"golang.org/x/time/rate"

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
}

// ProcessMonitor encapsulates the logic of processing events from an event
// monitor and translating them into actuator.ProcessesUpdate calls to the
// actuator.
type ProcessMonitor struct {
	handler    Handler
	procfsRoot string

	eventsCh chan event
	doneCh   chan struct{}

	wg           sync.WaitGroup
	shutdownOnce sync.Once
}

// NewProcessMonitor creates a new ProcessMonitor that will send updates to the
// given Actuator.
func NewProcessMonitor(h Handler) *ProcessMonitor {
	return newProcessMonitor(h, kernel.ProcFSRoot())
}

// NotifyExec is a callback to notify the monitor that a process has started.
func (pm *ProcessMonitor) NotifyExec(pid uint32) {
	pm.sendEvent(&processEvent{kind: processEventKindExec, pid: pid})
}

// NotifyExit is a callback to notify the monitor that a process has exited.
func (pm *ProcessMonitor) NotifyExit(pid uint32) {
	pm.sendEvent(&processEvent{kind: processEventKindExit, pid: pid})
}

// newProcessMonitor is injectable with a fake FS for tests.
func newProcessMonitor(h Handler, procFS string) *ProcessMonitor {
	pm := &ProcessMonitor{
		handler:    h,
		procfsRoot: procFS,
		eventsCh:   make(chan event, 1024),
		doneCh:     make(chan struct{}),
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
		pa, err := analyzeProcess(pid, pm.procfsRoot)
		if err != nil && analysisFailureLogLimiter.Allow() {
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
