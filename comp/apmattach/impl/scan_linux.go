// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apmattachimpl

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const javaExeName = "java"

// procKey identifies a process across PID reuse.
type procKey struct {
	pid       int
	startTime uint64
}

type candidate struct {
	key procKey
	exe string
}

func listJavaProcesses(procRoot string) ([]candidate, error) {
	pids, err := kernel.AllPidsProcs(procRoot)
	if err != nil {
		return nil, err
	}
	var out []candidate
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		exe, ok := processExe(procRoot, pid)
		if !ok || filepath.Base(exe) != javaExeName {
			continue
		}
		startTime, err := processStartTime(procRoot, pid)
		if err != nil {
			continue
		}
		out = append(out, candidate{key: procKey{pid: pid, startTime: startTime}, exe: exe})
	}
	return out, nil
}

func processExe(procRoot string, pid int) (string, bool) {
	dir := filepath.Join(procRoot, strconv.Itoa(pid))
	if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
		return strings.TrimSuffix(exe, " (deleted)"), true
	}
	if cmdline, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil && len(cmdline) > 0 {
		if argv0, _, _ := bytes.Cut(cmdline, []byte{0}); len(argv0) > 0 {
			return string(argv0), true
		}
	}
	if comm, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
		if name := strings.TrimSpace(string(comm)); name != "" {
			return name, true
		}
	}
	return "", false
}

func processStartTime(procRoot string, pid int) (uint64, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	return parseStatStartTime(data)
}

func parseStatStartTime(data []byte) (uint64, error) {
	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return 0, errors.New("malformed stat: no command name")
	}
	fields := strings.Fields(string(data[end+1:]))
	const startTimeField, firstFieldAfterComm = 22, 3
	const startTimeIdx = startTimeField - firstFieldAfterComm
	if len(fields) <= startTimeIdx {
		return 0, fmt.Errorf("malformed stat: %d fields after command name", len(fields))
	}
	return strconv.ParseUint(fields[startTimeIdx], 10, 64)
}
