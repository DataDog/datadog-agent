// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// These helpers exist so each node's size() can approximate its real heap footprint
// (backing arrays, map buckets, string content) instead of only the shallow struct size.
// The result is still an estimate — Go does not expose map bucket sizes — but it tracks
// real RAM usage closely enough to drive the `ActivityDumpMaxDumpSize` budget and the
// per-profile `storage:ram` metric without lying to operators.
//
// Constants here are intentionally conservative: when in doubt we round up so the
// metric overshoots a little rather than undershooting by 5-10x.
const (
	stringHeaderBytes = int64(unsafe.Sizeof(""))
	// mapHeaderBytes approximates the runtime.hmap struct (count, flags, hash0, buckets pointer, ...).
	mapHeaderBytes = int64(48)
	// mapPerEntryBytes accounts for bucket+tophash+padding overhead Go layers on top of K+V.
	// Real value depends on bucket fill but ~16 bytes per entry is a reasonable middle ground.
	mapPerEntryBytes = int64(16)
)

// stringSliceBytes returns the heap bytes used by a []string: backing array slots + content.
func stringSliceBytes(ss []string) int64 {
	if cap(ss) == 0 {
		return 0
	}
	n := int64(cap(ss)) * stringHeaderBytes
	for _, s := range ss {
		n += int64(len(s))
	}
	return n
}

// stringMapBytes approximates the heap used by a map[string]V where V is a pointer.
// Each entry pays for the key string header+content and a pointer-sized value slot.
func stringMapBytes[V any](m map[string]V) int64 {
	if len(m) == 0 {
		return 0
	}
	valSize := int64(unsafe.Sizeof(*new(V)))
	n := mapHeaderBytes + int64(len(m))*(stringHeaderBytes+valSize+mapPerEntryBytes)
	for k := range m {
		n += int64(len(k))
	}
	return n
}

// fixedKeyMapBytes approximates the heap used by a map with a fixed-size key (struct or
// scalar — no heap pointers in K). Used for maps keyed by e.g. model.IMDSEvent or
// model.FiveTuple where the key has no allocated content.
func fixedKeyMapBytes[K comparable, V any](m map[K]V) int64 {
	if len(m) == 0 {
		return 0
	}
	keySize := int64(unsafe.Sizeof(*new(K)))
	valSize := int64(unsafe.Sizeof(*new(V)))
	return mapHeaderBytes + int64(len(m))*(keySize+valSize+mapPerEntryBytes)
}

// sliceBackingBytes returns the heap bytes used by a slice's backing array, excluding
// anything the elements themselves point at. Pass `unsafe.Sizeof(elem)` for the element size.
func sliceBackingBytes(slcCap int, elemSize uintptr) int64 {
	return int64(slcCap) * int64(elemSize)
}

// seenBytes returns the heap bytes used by a NodeBase's seen slice.
func seenBytes(b NodeBase) int64 {
	return int64(cap(b.seen)) * int64(unsafe.Sizeof(seenEntry{}))
}

// fileEventStringsBytes counts every heap-allocated string carried by a model.FileEvent
// (path/basename/filesystem/package metadata + hashes backing). Shared by the main exec
// FileEvent and the LinuxBinprm interpreter FileEvent so both contribute the same way.
func fileEventStringsBytes(fe *model.FileEvent) int64 {
	var n int64
	n += int64(len(fe.PathnameStr))
	n += int64(len(fe.BasenameStr))
	n += int64(len(fe.Filesystem))
	n += int64(len(fe.PkgName))
	n += int64(len(fe.PkgVersion))
	n += stringSliceBytes(fe.Hashes)
	return n
}

// processStringsBytes counts the major string allocations on a model.Process. These are
// the fields that actually consume non-trivial heap on long-running workloads — full argv,
// env vars, container tags, credentials labels, etc. — and that the previous shallow
// size() ignored.
func processStringsBytes(p *model.Process) int64 {
	var n int64
	n += fileEventStringsBytes(&p.FileEvent)
	n += fileEventStringsBytes(&p.LinuxBinprm.FileEvent)

	n += int64(len(p.Argv0))
	n += int64(len(p.Comm))
	n += int64(len(p.TTYName))
	n += stringSliceBytes(p.Argv)
	n += stringSliceBytes(p.Envs)
	n += stringSliceBytes(p.Envp)

	n += int64(len(p.ContainerContext.ContainerID))
	n += stringSliceBytes(p.ContainerContext.Tags)
	n += int64(len(p.CGroup.CGroupID))

	n += int64(len(p.Credentials.User))
	n += int64(len(p.Credentials.Group))
	n += int64(len(p.Credentials.EUser))
	n += int64(len(p.Credentials.EGroup))
	n += int64(len(p.Credentials.FSUser))
	n += int64(len(p.Credentials.FSGroup))
	return n
}
