// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds ptracer related files
package ptracer

// SyscallState defines the state of the syscall
type SyscallState struct {
	Entry bool
	Exec  bool
}

// SyscallStateTracker defines a syscall state tracker
type SyscallStateTracker struct {
	states map[int]*SyscallState
}

// NewSyscallStateTracker returns a new syscall state tracker
func NewSyscallStateTracker() *SyscallStateTracker {
	return &SyscallStateTracker{
		states: make(map[int]*SyscallState),
	}
}

// IsSyscallEntry returns true is the pid is at a syscall entry
func (st *SyscallStateTracker) IsSyscallEntry(pid int) bool {
	return st.states[pid].Entry
}

// NextStop update the state for the given pid
func (st *SyscallStateTracker) NextStop(pid int) *SyscallState {
	state, exists := st.states[pid]
	if exists {
		state.Entry = !state.Entry
	} else {
		state = &SyscallState{
			Entry: true,
		}
		st.states[pid] = state
	}

	return state
}

// Exit delete the pid from the tracker
func (st *SyscallStateTracker) Exit(pid int) {
	delete(st.states, pid)
}

// PeekState return the state of the given pid
func (st *SyscallStateTracker) PeekState(pid int) *SyscallState {
	return st.states[pid]
}
