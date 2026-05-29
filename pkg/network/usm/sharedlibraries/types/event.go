// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

// LibraryOpenEvent represents a shared library open event with the resolved path and PID.
// This type is used in the SharedLibraryWatcher interface to avoid circular dependencies
// between pkg/ebpf/uprobes and pkg/network/usm/sharedlibraries.
type LibraryOpenEvent struct {
	Path string
	Pid  uint32
}
