// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sharedconsts holds model related shared constants
package sharedconsts

// ExitCause represents the cause of a process termination
type ExitCause uint32

func (cause ExitCause) String() string {
	switch cause {
	case ExitExited:
		return "EXITED"
	case ExitCoreDumped:
		return "COREDUMPED"
	case ExitSignaled:
		return "SIGNALED"
	default:
		return "UNKNOWN"
	}
}

const (
	// ExitExited Process exited normally
	ExitExited ExitCause = iota
	// ExitCoreDumped Process was terminated with a coredump signal
	ExitCoreDumped
	// ExitSignaled Process was terminated with a signal other than a coredump
	ExitSignaled
)
