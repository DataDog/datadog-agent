// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package uprobes

import (
	sharedlibtypes "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/types"
)

// SharedLibraryWatcher abstracts the shared library eBPF monitoring program.
// This interface breaks the circular dependency between pkg/ebpf/uprobes and
// pkg/network/usm/sharedlibraries (which imports pkg/ebpf).
// The concrete implementation is sharedlibraries.EbpfProgram.
type SharedLibraryWatcher interface {
	// InitWithLibsets initializes the eBPF program for the given library sets.
	InitWithLibsets(libsets ...sharedlibtypes.Libset) error

	// SubscribeWithPath registers a callback for library open events (path string + pid).
	// Returns an unsubscribe function.
	SubscribeWithPath(callback func(sharedlibtypes.LibraryOpenEvent), libsets ...sharedlibtypes.Libset) (func(), error)

	// Stop stops the shared library watcher.
	Stop()

	// IsSupported returns true if shared library monitoring is available.
	IsSupported() bool
}
