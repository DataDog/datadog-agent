// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"golang.org/x/sys/unix"
)

// procPidInitIno is the inode number of the root PID namespace
// (PROC_PID_INIT_INO in the kernel).
const procPidInitIno = 0xEFFFFFFC

// pidNamespaceConstant returns the value of the jvm_pidns_ino constant of the
// eBPF program for the procfs mounted at procRoot: 0 when the PID namespace of
// that procfs is the root PID namespace, the inode number of that namespace
// otherwise.
//
// The eBPF program reports the tgids in that namespace, so that they match
// the PIDs of procRoot: in kind, for instance, system-probe reads the procfs
// of the kind node, whose PID namespace is not the root one.
func pidNamespaceConstant(procRoot string) (uint64, error) {
	var st unix.Stat_t
	// /proc/1 is the init process of the PID namespace of the procfs
	if err := unix.Stat(procPidPath(procRoot, 1, "ns", "pid"), &st); err != nil {
		return 0, err
	}
	if st.Ino == procPidInitIno {
		return 0, nil
	}
	return st.Ino, nil
}
