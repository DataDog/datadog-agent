// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils groups multiple utils function that can be used by the secl package
package utils

import "sync"

// SampledIgnoredSyscallNames are high-frequency, low-signal syscalls the v2 sampler
// fast-exits in-kernel; their ids are seeded back into a profile's reported syscall list.
var SampledIgnoredSyscallNames = []string{
	"read", "write", "readv", "writev", "pread64", "pwrite64", "preadv", "pwritev",
	"recvfrom", "sendto", "recvmsg", "sendmsg", "recvmmsg", "sendmmsg",
	"futex", "poll", "ppoll", "select", "pselect6", "epoll_wait", "epoll_pwait",
	"nanosleep",
}

var (
	sampledIgnoredOnce sync.Once
	sampledIgnoredIDs  map[string][]int
)

func buildSampledIgnored() {
	nameSet := make(map[string]struct{}, len(SampledIgnoredSyscallNames))
	for _, n := range SampledIgnoredSyscallNames {
		nameSet[n] = struct{}{}
	}
	sampledIgnoredIDs = make(map[string][]int)
	for key, name := range Syscalls {
		if _, ok := nameSet[name]; !ok {
			continue
		}
		sampledIgnoredIDs[key.Arch] = append(sampledIgnoredIDs[key.Arch], key.ID)
	}
}

// SampledIgnoredSyscallIDsForArch returns the syscall ids of the sampler ignore list
// that exist on the given arch ("amd64"/"arm64").
func SampledIgnoredSyscallIDsForArch(arch string) []int {
	sampledIgnoredOnce.Do(buildSampledIgnored)
	return sampledIgnoredIDs[arch]
}
