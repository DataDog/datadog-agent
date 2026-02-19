// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type processState struct {
	procRuntimeID
	executable      actuator.Executable
	symbolicator    *refCountedSymbolicator
	symbolicatorErr error
	gitInfo         process.GitInfo
	containerInfo   process.ContainerInfo
}

type processStore struct {
	mu              sync.Mutex
	processes       map[actuator.ProcessID]*processState
	processByProgID map[ir.ProgramID]actuator.ProcessID
}

func newProcessStore() *processStore {
	return &processStore{
		processes:       make(map[actuator.ProcessID]*processState),
		processByProgID: make(map[ir.ProgramID]actuator.ProcessID),
	}
}

func (ps *processStore) remove(removals []process.ID, dm *diagnosticsManager) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for _, pid := range removals {
		if state, ok := ps.processes[pid]; ok {
			if state.symbolicator != nil {
				if err := state.symbolicator.Close(); err != nil {
					log.Warnf("error closing symbolicator for process %v: %v", pid, err)
				}
			}
			delete(ps.processes, pid)
			dm.remove(state.procRuntimeID.runtimeID)
		}
	}
}

func (ps *processStore) ensureExists(update *process.Config) procRuntimeID {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	proc, ok := ps.processes[update.ProcessID]
	if !ok {
		proc = &processState{
			procRuntimeID: procRuntimeID{
				ID:          update.ProcessID,
				runtimeID:   update.RuntimeID,
				service:     update.Service,
				version:     update.Version,
				environment: update.Environment,
			},
			executable:    update.Executable,
			gitInfo:       update.GitInfo,
			containerInfo: update.Container,
		}
		if update.GitInfo != (process.GitInfo{}) {
			proc.procRuntimeID.gitInfo = &proc.gitInfo
		}
		if update.Container != (process.ContainerInfo{}) {
			proc.procRuntimeID.containerInfo = &proc.containerInfo
		}
		ps.processes[update.ProcessID] = proc
		return proc.procRuntimeID
	}
	// Update existing metadata with the latest information.
	if update.Service != "" {
		proc.service = update.Service
	}
	if update.Version != "" {
		proc.version = update.Version
	}
	if update.Environment != "" {
		proc.environment = update.Environment
	}
	if update.RuntimeID != "" {
		proc.runtimeID = update.RuntimeID
	}
	if update.Executable.Path != "" {
		proc.executable = update.Executable
	}
	if update.GitInfo != (process.GitInfo{}) {
		proc.gitInfo = update.GitInfo
		proc.procRuntimeID.gitInfo = &proc.gitInfo
	}
	if update.Container != (process.ContainerInfo{}) {
		proc.containerInfo = update.Container
		proc.procRuntimeID.containerInfo = &proc.containerInfo
	}
	return proc.procRuntimeID
}

func (ps *processStore) getSymbolicator(progID ir.ProgramID) symbol.Symbolicator {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	procID, ok := ps.processByProgID[progID]
	if !ok {
		return noopSymbolicator{}
	}
	proc, ok := ps.processes[procID]
	if !ok {
		return noopSymbolicator{}
	}
	if proc.symbolicator != nil {
		proc.symbolicator.addRef()
		return proc.symbolicator
	}
	if proc.symbolicatorErr != nil {
		return noopSymbolicator{}
	}

	inner, file, err := newSymbolicator(proc.executable)
	proc.symbolicatorErr = err
	if err != nil {
		log.Warnf("error creating symbolicator for %v: %v", proc.executable, err)
		return noopSymbolicator{}
	}
	// refCount starts at 1 for the processState's own reference.
	// addRef adds the caller's (sink's) reference.
	proc.symbolicator = newRefCountedSymbolicator(inner, file)
	proc.symbolicator.addRef()
	return proc.symbolicator
}

func (ps *processStore) link(programID ir.ProgramID, procID actuator.ProcessID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.processByProgID[programID] = procID
}

func (ps *processStore) unlink(programID ir.ProgramID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.processByProgID, programID)
}

func (ps *processStore) updateOnLoad(
	procID actuator.ProcessID,
	executable actuator.Executable,
	programID ir.ProgramID,
) (procRuntimeID, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p, ok := ps.processes[procID]
	if !ok {
		return procRuntimeID{}, false
	}
	p.executable = executable
	ps.processByProgID[programID] = procID
	return p.procRuntimeID, true
}
