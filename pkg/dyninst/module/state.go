// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"io"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type processState struct {
	procRuntimeID
	executable       actuator.Executable
	symbolicator     symbol.Symbolicator
	symbolicatorFile io.Closer
	symbolicatorErr  error
	gitInfo          procmon.GitInfo
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

func (ps *processStore) remove(removals []procmon.ProcessID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for _, pid := range removals {
		if state, ok := ps.processes[pid]; ok {
			if state.symbolicatorFile != nil {
				if err := state.symbolicatorFile.Close(); err != nil {
					log.Warnf("error closing symbolicator file for process %v: %v", pid, err)
				}
			}
			delete(ps.processes, pid)
		}
	}
}

func (ps *processStore) ensureExists(update *rcscrape.ProcessUpdate) procRuntimeID {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	proc, ok := ps.processes[update.ProcessID]
	if !ok {
		proc = &processState{
			procRuntimeID: procRuntimeID{
				ProcessID: update.ProcessID,
				runtimeID: update.RuntimeID,
				service:   update.Service,
			},
			executable: update.Executable,
			gitInfo:    update.GitInfo,
		}
		if update.GitInfo != (procmon.GitInfo{}) {
			proc.procRuntimeID.gitInfo = &proc.gitInfo
		}
		if update.Container != (procmon.ContainerInfo{}) {
			proc.procRuntimeID.containerInfo = &update.Container
		}
		ps.processes[update.ProcessID] = proc
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
		return proc.symbolicator
	}

	proc.symbolicator, proc.symbolicatorFile, proc.symbolicatorErr = newSymbolicator(proc.executable)
	if proc.symbolicatorErr != nil {
		log.Warnf("error creating symbolicator for %v: %v", proc.executable, proc.symbolicatorErr)
		proc.symbolicator = noopSymbolicator{}
	}
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

func (ps *processStore) getRuntimeID(procID actuator.ProcessID) (procRuntimeID, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p, ok := ps.processes[procID]
	if !ok {
		return procRuntimeID{}, false
	}
	return p.procRuntimeID, true
}
