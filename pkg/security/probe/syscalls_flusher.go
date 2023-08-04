// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"time"
	_ "unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/cilium/ebpf"
)

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

// FlushSyscalls does a best effort flush of the in-flight syscalls map
func FlushSyscalls(syscallsMaps *ebpf.Map) {
	seclog.Debugf("Flushing in-flight syscalls")

	if syscallsMaps == nil {
		seclog.Errorf("flushing syscalls: nil-map")
		return
	}

	var (
		key   uint64
		value struct {
			Time      int64
			EventType uint64
		}
		iter = syscallsMaps.Iterate()
	)

	startFlushTime := nanotime()
	seclog.Errorf("begin flushing: time = %d", startFlushTime)

	// iteration in eBPF is a difficult subject since the map can be edited by the kernel side
	// at the same time
	// to resolve this issue we ignore errors
	cleaned := 0
	for iter.Next(&key, &value) {
		seclog.Errorf("syscall flushing: %v %d", value.EventType, value.Time)

		if value.Time != 0 && value.Time <= startFlushTime-int64(10*time.Second) {
			// ignore error
			if err := syscallsMaps.Delete(key); err == nil {
				cleaned++
			}
		}
	}

	if err := iter.Err(); err != nil {
		seclog.Errorf("syscall flushing encoutered an error: %v", err)
	}
	seclog.Errorf("syscall flushing: cleaned %d", cleaned)
}
