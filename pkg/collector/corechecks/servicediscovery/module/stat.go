// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"bufio"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// pageSize stores the page size of the system in bytes, since the values in
// statm are in pages.
var pageSize = uint64(os.Getpagesize())

// getRSS returns the RSS for the process, in bytes. Compare MemoryInfo() in
// gopsutil which does the same thing but which parses several other fields
// which we're not interested in.
func getRSS(proc *process.Process) (uint64, error) {
	statmPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "statm")

	// This file is very small so just read it fully.
	contents, err := os.ReadFile(statmPath)
	if err != nil {
		return 0, err
	}

	// See proc(5) for a description of the format of statm and the fields.
	fields := strings.Split(string(contents), " ")
	if len(fields) < 6 {
		return 0, errors.New("invalid statm")
	}

	rssPages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}

	return rssPages * pageSize, nil
}

func getGlobalCPUTime() (uint64, error) {
	globalStatPath := kernel.HostProc("stat")

	// This file is very small so just read it fully.
	file, err := os.Open(globalStatPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Try to read the first line; it contains all the info we need.
	for scanner.Scan() {
		// See proc(5) for a description of the format of statm and the fields.
		fields := strings.Fields(scanner.Text())
		if fields[0] != "cpu" {
			continue
		}

		var totalTime uint64
		for _, field := range fields[1:] {
			val, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return 0, err
			}
			totalTime += val
		}

		return totalTime, nil
	}

	return 0, scanner.Err()
}

func updateCPUUsageStats(proc *process.Process, info *serviceInfo, lastGlobalCPUTime, currentGlobalCPUTime uint64) (float64, error) {
	statPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "stat")

	// This file is very small so just read it fully.
	contents, err := os.ReadFile(statPath)
	if err != nil {
		return 0, err
	}

	// See proc(5) for a description of the format of statm and the fields.
	fields := strings.Fields(string(contents))
	if len(fields) < 52 {
		return 0, errors.New("invalid stat")
	}

	// Parse fields at index 15 and 16, resp. User and System CPU time.
	// See proc_pid_stat(5), for details.
	usrTime, err := strconv.ParseUint(fields[13], 10, 64)
	if err != nil {
		return 0, err
	}

	sysTime, err := strconv.ParseUint(fields[14], 10, 64)
	if err != nil {
		return 0, err
	}

	process_time_delta := float64(usrTime + sysTime - info.cpuTime)
	global_time_delta := float64(currentGlobalCPUTime - lastGlobalCPUTime)
	cpu_usage := process_time_delta / global_time_delta * float64(runtime.NumCPU())

	info.cpuTime = usrTime + sysTime

	return cpu_usage, nil
}
