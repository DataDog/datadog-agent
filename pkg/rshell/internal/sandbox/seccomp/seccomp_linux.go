// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package seccomp

import (
	"fmt"
	"runtime"

	elasticseccomp "github.com/elastic/go-seccomp-bpf"
)

// Restrict installs a default-allow seccomp filter that returns EPERM for the
// named syscalls. clone is the one exception: it returns EPERM unless its flags
// exactly match the Go runtime's thread-creation flags. The filter is
// synchronized to every existing thread and sets no_new_privs before it is
// loaded.
//
// A syscall that exists on a supported architecture but not on the current
// one is ignored because it cannot be invoked with the current ABI. Unknown
// names and duplicate entries are rejected instead of silently weakening the
// policy.
func Restrict(denied []string) error {
	group, err := syscallGroupForArchitecture(runtime.GOARCH, denied)
	if err != nil {
		return err
	}

	filter := elasticseccomp.Filter{
		NoNewPrivs: true,
		Flag:       elasticseccomp.FilterFlagTSync,
		Policy: elasticseccomp.Policy{
			DefaultAction: elasticseccomp.ActionAllow,
			Syscalls:      []elasticseccomp.SyscallGroup{group},
		},
	}
	if err := elasticseccomp.LoadFilter(filter); err != nil {
		return fmt.Errorf("install seccomp filter: %w", err)
	}
	return nil
}
