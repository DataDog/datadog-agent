// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package sysinfo

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	uptimePath  = "/proc/uptime"
	loadavgPath = "/proc/loadavg"
)

const (
	// maxProcFileSize caps reads from /proc pseudo-files. Both files are always
	// well under 128 bytes, but a malicious FUSE overlay could return arbitrary
	// data — the cap bounds the memory allocation.
	maxProcFileSize = 128
)

func getImpl() (Info, error) {
	uptimeSecs, err := readUptimeSeconds()
	if err != nil {
		return Info{}, err
	}

	load1, load5, load15, err := readLoadAvg()
	if err != nil {
		return Info{}, err
	}

	return Info{
		UptimeSeconds: uptimeSecs,
		Load1:         load1,
		Load5:         load5,
		Load15:        load15,
		LoadAvailable: true,
		BootTime:      time.Now().Unix() - int64(uptimeSecs),
	}, nil
}

func readUptimeSeconds() (float64, error) {
	f, err := os.Open(uptimePath)
	if err != nil {
		return 0, fmt.Errorf("sysinfo: open %s: %w", uptimePath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxProcFileSize))
	if err != nil {
		return 0, fmt.Errorf("sysinfo: read %s: %w", uptimePath, err)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("sysinfo: unexpected format in %s", uptimePath)
	}

	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("sysinfo: parse %s: %w", uptimePath, err)
	}
	return secs, nil
}

func readLoadAvg() (load1, load5, load15 float64, err error) {
	f, err := os.Open(loadavgPath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("sysinfo: open %s: %w", loadavgPath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxProcFileSize))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("sysinfo: read %s: %w", loadavgPath, err)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("sysinfo: unexpected format in %s", loadavgPath)
	}

	parse := func(s string) (float64, error) {
		v, e := strconv.ParseFloat(s, 64)
		if e != nil {
			return 0, fmt.Errorf("sysinfo: parse %s: %w", loadavgPath, e)
		}
		return v, nil
	}

	if load1, err = parse(fields[0]); err != nil {
		return 0, 0, 0, err
	}
	if load5, err = parse(fields[1]); err != nil {
		return 0, 0, 0, err
	}
	if load15, err = parse(fields[2]); err != nil {
		return 0, 0, 0, err
	}
	return load1, load5, load15, nil
}
