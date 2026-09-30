// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

// ExecFile is what the consumer's Copy() extracts from an exec event. It is built on the
// event hot path, so it must stay small and must not require any I/O to fill.
type ExecFile struct {
	PID         uint32
	CGroupID    containerutils.CGroupID
	ContainerID containerutils.ContainerID
	// Path is the executed file path (Process.FileEvent.PathnameStr)
	Path string
	// MountID is FileEvent.PathKey.MountID
	MountID uint32
	// Inode is FileEvent.PathKey.Inode
	Inode uint64
	// CTime is FileEvent.FileFields.CTime
	CTime uint64
	// Filesystem is the filesystem type of the file, used to bypass the identity cache on FUSE
	// and network filesystems. It may be empty if it wasn't cheap to resolve in Copy().
	Filesystem string
	// IsScript is true for the script file of an interpreter exec (LinuxBinprm.FileEvent).
	// /proc/<pid>/exe points at the interpreter for those, so it must not be used to open them.
	IsScript bool
	// SeenAt is when the consumer received the event
	SeenAt time.Time
}

// Identity returns the identity key of the file
func (f *ExecFile) Identity() Identity {
	return Identity{
		MountID: f.MountID,
		Inode:   f.Inode,
		CTime:   f.CTime,
	}
}

// Identity identifies a file version without reading it. Any content write changes the ctime,
// and an unprivileged user can't set it.
type Identity struct {
	MountID uint32
	Inode   uint64
	CTime   uint64
}

// Deduper holds both dedupe levels: file identity, then content hash.
// Implementations must be safe for concurrent use.
type Deduper interface {
	// IdentityFresh returns true when the identity was checked recently enough that the file
	// can be skipped without any I/O
	IdentityFresh(id Identity, now time.Time) bool
	// MarkIdentity records that the identity was checked at now
	MarkIdentity(id Identity, now time.Time)
	// ClaimHash returns true when the caller now owns the scan of this content. It returns false
	// when the content was already scanned, or is being scanned.
	ClaimHash(sum [32]byte) bool
	// ReleaseHash forgets a claimed hash, so that a later exec retries the scan. It must be called
	// when a claimed scan is dropped or fails.
	ReleaseHash(sum [32]byte)
}

// Match is a single YARA rule match
type Match struct {
	Rule      string
	Namespace string
	Tags      []string
}

// Scanner wraps the YARA engine. Rules are compiled once at startup.
// Implementations must be safe for concurrent use.
type Scanner interface {
	// Scan scans data, and must return when ctx is done
	Scan(ctx context.Context, data []byte) ([]Match, error)
	// RulesVersion identifies the compiled ruleset, and is included in every report
	RulesVersion() string
}

// Reporter emits scan results.
// Implementations must be safe for concurrent use.
type Reporter interface {
	// Report is called once per completed scan attempt. err is set when the scan failed.
	Report(f ExecFile, sum [32]byte, matches []Match, err error)
}

// ScanJob is a file content ready to be scanned, handed from the file reader to the ScanPool.
// Data is read once and was hashed into Sum, so the scanned bytes are exactly the hashed bytes.
type ScanJob struct {
	File ExecFile
	Sum  [32]byte
	Data []byte
	// Done must be called exactly once when the pool is finished with Data, whether the job was
	// scanned, dropped or failed. It returns the buffer to its pool. It may be nil.
	Done func()
}

// ScanPool runs scans on its own bounded set of workers
type ScanPool interface {
	// Submit queues a job without blocking. It returns false when the queue is full; the job is
	// then dropped, and the pool has already released its hash and called its Done.
	Submit(job ScanJob) bool
}
