// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package coredump

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// nameSpecifiers are the core_pattern specifiers that the Linux kernel
// replaces with the binary name: %e is the comm (15 characters max), %E is
// the executable path with '/' replaced by '!', and %f is the executable file
// name (Linux 5.9 and later). %h is the host name: it is not a binary name,
// but knowing it keeps the match exact.
var nameSpecifiers = map[byte]int{'e': specName, 'E': specPath, 'f': specFile, 'h': specHost}

func readCorePattern() (string, error) {
	b, err := os.ReadFile("/proc/sys/kernel/core_pattern")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\n"), nil
}

// procName returns the comm of the process, that the kernel uses for %e.
func procName() string {
	if b, err := os.ReadFile("/proc/self/comm"); err == nil {
		if name := strings.TrimRight(string(b), "\n"); name != "" {
			return name
		}
	}
	name := filepath.Base(os.Args[0])
	if len(name) > 15 {
		name = name[:15]
	}
	return name
}

func freeBytes(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}

func setCoreLimit(cur, maxLimit uint64) error {
	return unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: cur, Max: maxLimit})
}

func getCoreLimit() (uint64, uint64, error) {
	var rl unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &rl); err != nil {
		return 0, 0, err
	}
	return rl.Cur, rl.Max, nil
}
