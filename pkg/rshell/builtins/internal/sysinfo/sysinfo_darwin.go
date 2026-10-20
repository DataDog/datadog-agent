// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package sysinfo

import (
	"encoding/binary"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// darwinFscale is the fixed-point scale factor used by the Darwin kernel for
// load averages. It is the compile-time constant FSCALE from <sys/param.h>
// and has been 2048 on all Darwin/XNU versions. We hardcode it to avoid
// parsing the variable-width fscale field of struct loadavg.
const darwinFscale = 2048

func getImpl() (Info, error) {
	bootSec, err := readBootTime()
	if err != nil {
		return Info{}, err
	}

	load1, load5, load15, err := readLoadAvg()
	if err != nil {
		return Info{}, err
	}

	uptimeSecs := float64(time.Now().Unix() - bootSec)

	return Info{
		UptimeSeconds: uptimeSecs,
		Load1:         load1,
		Load5:         load5,
		Load15:        load15,
		LoadAvailable: true,
		BootTime:      bootSec,
	}, nil
}

// readBootTime reads kern.boottime via sysctl. On 64-bit Darwin the kernel
// returns a struct timeval: { int64 tv_sec, int32 tv_usec } = 12 bytes.
// We only need tv_sec (the first 8 bytes).
func readBootTime() (int64, error) {
	data, err := unix.SysctlRaw("kern.boottime")
	if err != nil {
		return 0, fmt.Errorf("sysinfo: sysctl kern.boottime: %w", err)
	}
	if len(data) < 8 {
		return 0, fmt.Errorf("sysinfo: kern.boottime: short response (%d bytes)", len(data))
	}
	// tv_sec occupies the first 8 bytes, little-endian on all Apple hardware
	// (both Intel x86-64 and Apple Silicon arm64 are little-endian).
	tvSec := int64(binary.LittleEndian.Uint64(data[0:8]))
	return tvSec, nil
}

// readLoadAvg reads vm.loadavg via sysctl. On 64-bit Darwin the kernel returns
// struct loadavg: { fixpt_t ldavg[3], long fscale } where fixpt_t = uint32 and
// long = int64. The three load values occupy bytes 0–11; we ignore fscale and
// divide by the well-known compile-time constant darwinFscale instead.
func readLoadAvg() (load1, load5, load15 float64, err error) {
	data, err := unix.SysctlRaw("vm.loadavg")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("sysinfo: sysctl vm.loadavg: %w", err)
	}
	if len(data) < 12 {
		return 0, 0, 0, fmt.Errorf("sysinfo: vm.loadavg: short response (%d bytes)", len(data))
	}
	// Each ldavg field is a uint32 at offsets 0, 4, 8 (little-endian).
	l1 := binary.LittleEndian.Uint32(data[0:4])
	l5 := binary.LittleEndian.Uint32(data[4:8])
	l15 := binary.LittleEndian.Uint32(data[8:12])
	return float64(l1) / darwinFscale, float64(l5) / darwinFscale, float64(l15) / darwinFscale, nil
}
