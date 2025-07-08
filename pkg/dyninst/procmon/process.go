// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"fmt"
	"syscall"
)

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

// FileKey identifies a file on a device, and the time it was
// last modified.
type FileKey struct {
	// The device and inode of the file.
	FileHandle
	// The time the file was last modified.
	LastModified syscall.Timespec
}

// String returns a string representation of the file key.
func (k FileKey) String() string {
	h, c := k.FileHandle, k.LastModified
	return fmt.Sprintf("%d.%dm%d.%d", h.Dev, h.Ino, c.Sec, c.Nsec)
}

// String returns a string representation of the executable.
func (e Executable) String() string {
	return fmt.Sprintf("%s@%s", e.Path, e.Key)
}
