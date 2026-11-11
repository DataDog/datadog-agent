// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package seccomp

import (
	"errors"
	"fmt"
	"sort"

	elasticseccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
)

var supportedArchitectures = []string{"amd64", "arm64"}

// baseGoRuntimeCloneFlags is runtime.cloneFlags in Go's runtime/os_linux.go.
// The values are Linux's CLONE_VM, CLONE_FS, CLONE_FILES, CLONE_SIGHAND,
// CLONE_SYSVSEM, and CLONE_THREAD. The amd64 clone assembly adds CLONE_SETTLS
// when it creates an M; arm64 passes the base flags unchanged. A clone call
// with any other flags is process creation or an unreviewed thread
// configuration and is denied. Keep this synchronized when the Go toolchain is
// updated.
const (
	baseGoRuntimeCloneFlags = uint64(0x100 | 0x200 | 0x400 | 0x800 | 0x40000 | 0x10000)
	cloneSetTLS             = uint64(0x80000)
)

func syscallGroupForArchitecture(architecture string, denied []string) (elasticseccomp.SyscallGroup, error) {
	available, err := syscallsForArchitecture(architecture, denied)
	if err != nil {
		return elasticseccomp.SyscallGroup{}, err
	}
	if len(available) == 0 {
		return elasticseccomp.SyscallGroup{}, fmt.Errorf("seccomp denylist has no syscalls for %s", architecture)
	}

	group := elasticseccomp.SyscallGroup{Action: elasticseccomp.ActionErrno}
	for _, name := range available {
		if name == "clone" {
			cloneFlags, cloneErr := goRuntimeCloneFlagsForArchitecture(architecture)
			if cloneErr != nil {
				return elasticseccomp.SyscallGroup{}, cloneErr
			}
			group.NamesWithCondtions = append(group.NamesWithCondtions, elasticseccomp.NameWithConditions{
				Name: "clone",
				Conditions: elasticseccomp.ArgumentConditions{
					{
						Argument:  0,
						Operation: elasticseccomp.NotEqual,
						Value:     cloneFlags,
					},
				},
			})
			continue
		}
		group.Names = append(group.Names, name)
	}
	return group, nil
}

func goRuntimeCloneFlagsForArchitecture(architecture string) (uint64, error) {
	switch architecture {
	case "amd64":
		return baseGoRuntimeCloneFlags | cloneSetTLS, nil
	case "arm64":
		return baseGoRuntimeCloneFlags, nil
	default:
		return 0, fmt.Errorf("Go runtime clone flags: unsupported arch: %s", architecture)
	}
}

func syscallsForArchitecture(architecture string, denied []string) ([]string, error) {
	if !isSupportedArchitecture(architecture) {
		return nil, fmt.Errorf("validate seccomp architecture: unsupported arch: %s", architecture)
	}
	current, err := arch.GetInfo(architecture)
	if err != nil {
		return nil, fmt.Errorf("validate seccomp architecture: %w", err)
	}
	if len(denied) == 0 {
		return nil, errors.New("seccomp denylist must not be empty")
	}

	known := make(map[string]struct{})
	for _, supportedArchitecture := range supportedArchitectures {
		info, infoErr := arch.GetInfo(supportedArchitecture)
		if infoErr != nil {
			return nil, fmt.Errorf("load seccomp syscall table for %s: %w", supportedArchitecture, infoErr)
		}
		for name := range info.SyscallNames {
			known[name] = struct{}{}
		}
	}

	seen := make(map[string]struct{}, len(denied))
	available := make([]string, 0, len(denied))
	for _, name := range denied {
		if name == "" {
			return nil, errors.New("seccomp denylist contains an empty syscall name")
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("seccomp denylist contains duplicate syscall %q", name)
		}
		seen[name] = struct{}{}

		if _, exists := known[name]; !exists {
			return nil, fmt.Errorf("seccomp denylist contains unknown syscall %q", name)
		}
		if _, exists := current.SyscallNames[name]; exists {
			available = append(available, name)
		}
	}

	// Stable order makes assembled policy dumps and error investigations
	// reproducible even if callers construct their list from a map.
	sort.Strings(available)
	return available, nil
}

func isSupportedArchitecture(architecture string) bool {
	for _, supported := range supportedArchitectures {
		if architecture == supported {
			return true
		}
	}
	return false
}
