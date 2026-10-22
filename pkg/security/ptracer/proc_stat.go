// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func readProcessStat(pid int) (ppid uint32, startTime uint64, err error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, 0, err
	}
	return parseProcessStat(string(data))
}

func parseProcessStat(data string) (ppid uint32, startTime uint64, err error) {
	// comm (field 2) can contain spaces, newlines and parentheses.
	end := strings.LastIndexByte(data, ')')
	if end < 0 {
		return 0, 0, errors.New("missing comm in process stat")
	}
	fields := strings.Fields(data[end+1:])
	// Relative to state (field 3), ppid is at index 1 and starttime at 19.
	if len(fields) <= 19 {
		return 0, 0, errors.New("missing fields in process stat")
	}
	parent, err := strconv.ParseUint(fields[1], 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid ppid in process stat: %w", err)
	}
	startTime, err = strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid starttime in process stat: %w", err)
	}
	return uint32(parent), startTime, nil
}

// Resolve the TGID before the parent's clone notification has necessarily been
// handled, so early thread events use the process's stream and birth identity.
func getPidTGID(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return pid
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "Tgid:"); ok {
			tgid, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
			if err == nil && tgid > 0 {
				return int(tgid)
			}
			break
		}
	}
	return pid
}

func getPidPPID(pid int) uint32 {
	ppid, _, err := readProcessStat(pid)
	if err != nil {
		// Missing proc metadata must not discard an otherwise valid exec.
		return 0
	}
	return ppid
}
