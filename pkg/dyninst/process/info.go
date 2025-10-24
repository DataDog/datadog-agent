// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package process provides shared process-related types used by procmon,
// rcscrape, and other dyninst components.
package process

import (
	"fmt"
	"syscall"
)

// Info captures the instrumentation metadata associated with a process.
type Info struct {
	ProcessID   ID
	Executable  Executable
	Service     string
	Version     string
	Environment string
	GitInfo     GitInfo
	Container   ContainerInfo
}

// ID is a unique identifier for a process.
type ID struct {
	// PID is the operating system process ID.
	PID int32
}

// String returns a string representation of the process ID.
func (p ID) String() string {
	return fmt.Sprintf("{PID:%d}", p.PID)
}

// Executable is a reference to an executable file that a process is running.
type Executable struct {
	// Path is the path to the executable file.
	Path string

	// Key is a unique identifier for the executable file.
	Key FileKey
}

// String returns a string representation of the executable.
func (e Executable) String() string {
	return fmt.Sprintf("%s@%s", e.Path, e.Key)
}

// FileHandle identifies a file on a device.
type FileHandle struct {
	Dev uint64
	Ino uint64
}

// FileKey identifies a file on a device, and the time it was last modified.
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

// GitInfo is information about the git repository and commit sha of the process.
type GitInfo struct {
	// CommitSha is the git commit sha of the process.
	CommitSha string
	// RepositoryURL is the git repository url of the process.
	RepositoryURL string
}

// ContainerInfo is information about the container the process is running in.
type ContainerInfo struct {
	// EntityID is the entity id of the process. It is either derived from the
	// container id or inode of the cgroup root.
	EntityID string
	// ContainerID is the container id of the process.
	ContainerID string
}
