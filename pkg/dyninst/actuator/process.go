// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"fmt"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// ProcessesUpdate is a set of updates to the actuator's state.
//
// Note that this does not represent a complete update to all of the
// process state, but rather an incremental update to the set of processes.
type ProcessesUpdate struct {
	// Processes is a list of updates to the processes's instrumentation
	// configuration.
	Processes []ProcessUpdate

	// Removals is a list of process IDs that are no longer being instrumented.
	Removals []ProcessID
}

// ProcessUpdate is an update to a process's instrumentation configuration.
type ProcessUpdate struct {
	ProcessID  ProcessID
	Executable Executable

	// Probes is the *complete* set of probes for the process.
	//
	// If a previous update contained a different set of probes, they
	// will be wholly replaced by the new set.
	Probes []ir.ProbeDefinition
}

// Executable is a reference to an executable file that a process is running.
type Executable struct {
	// Path is the path to the executable file.
	Path string

	// Key is a unique identifier for the executable file.
	Key FileKey
}

// ProcessID is a unique identifier for a process.
type ProcessID struct {
	// PID is the operating system process ID.
	PID int32

	// Service is the service name for the process.
	Service string
	// Realistically this should include something about the start time of
	// the process to be robust to PID wraparound. This is less of a problem
	// these days now that pids in linux are 32 bits, but technically it's
	// possible.
}

// String returns a string representation of the process ID.
func (p ProcessID) String() string {
	if p.Service == "" {
		return fmt.Sprintf("{PID:%d}", p.PID)
	}
	return fmt.Sprintf("{PID:%d,Svc:%s}", p.PID, p.Service)
}

// FileHandle identifies a file on a device.
type FileHandle struct {
	Dev uint64
	Ino uint64
}

// FileCookie identifies a file on a device, and the time it was
// last modified.
type FileCookie syscall.Timespec

// FileKey identifies a file on a device, and the time it was
// last modified.
type FileKey struct {
	// The device and inode of the file.
	FileHandle
	// The time the file was last modified.
	FileCookie
}

// String returns a string representation of the file key.
func (k FileKey) String() string {
	h, c := k.FileHandle, k.FileCookie
	return fmt.Sprintf("%d.%dm%d.%d", h.Dev, h.Ino, c.Sec, c.Nsec)
}

// String returns a string representation of the executable.
func (e Executable) String() string {
	return fmt.Sprintf("%s@%s", e.Path, e.Key)
}
