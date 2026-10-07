// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package coredump

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// nameSpecifiers are the kern.corefile specifiers that darwin replaces with the
// process name.
var nameSpecifiers = map[byte]int{'N': specName}

func readCorePattern() (string, error) {
	return unix.Sysctl("kern.corefile")
}

func procName() string {
	return filepath.Base(os.Args[0])
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
