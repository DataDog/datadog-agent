// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package common defines shared types and structures for privileged logs functionality.
package common

// SymlinkPolicy controls whether a file open follows or rejects symbolic links.
// Callers must always choose an explicit policy; the zero value is invalid.
type SymlinkPolicy int

const (
	// symlinkPolicyInvalid is the zero value.  Passing it to any open function
	// causes an error so that new call sites cannot silently get the wrong behaviour.
	symlinkPolicyInvalid SymlinkPolicy = iota
	// FollowSymlinks resolves symbolic links when opening a file.  Use for
	// file sources whose paths are admin-specified or come from the container
	// runtime (e.g. /var/log/pods/…).
	FollowSymlinks
	// RejectSymlinks opens every path component with O_NOFOLLOW so that any
	// symlink encountered causes an immediate error.  Use for file sources
	// whose paths are resolved from /proc/<pid>/fd by the process_log provider:
	// those paths are canonical at discovery time, so a symlink found later
	// indicates an attacker-controlled swap.
	RejectSymlinks
)
