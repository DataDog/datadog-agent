// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ebpf contains general eBPF related types and functions
package ebpf

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	manager "github.com/DataDog/ebpf-manager"
)

var (
	// ErrNotImplemented will be returned on non-linux environments like Windows and Mac OSX
	ErrNotImplemented = errors.New("BPF-based system probe not implemented on non-linux systems")
)

// GetPatchedPrintkEditor returns a ConstantEditor that patches log_debug calls to always print one newline,
// independently of whether bpf_trace_printk adds its own (from Linux 5.9 onwards) or not.
func GetPatchedPrintkEditor() manager.ConstantEditor {
	// The default is to add a newline: better to have two newlines than none.
	lastCharacter := '\n'
	kernelVersion, err := kernel.HostVersion()

	// if we can detect the kernel version and it's later than 5.9.0, set the last
	// character to '\0', deleting the newline added in the log_debug call
	if err == nil && kernelVersion >= kernel.VersionCode(5, 9, 0) {
		lastCharacter = 0 // '\0'
	}

	return manager.ConstantEditor{
		Name:          "log_debug_last_character",
		Value:         uint64(lastCharacter),
		FailOnMissing: false, // No problem if the constant is not there, that just means the log_debug method wasn't used or it isn't a debug build
	}
}
