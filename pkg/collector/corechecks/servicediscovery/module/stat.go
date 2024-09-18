// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"bufio"
	"bytes"
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
	if !scanner.Scan() {
		return 0, scanner.Err()
	}

	// See proc(5) for a description of the format of statm and the fields.
	fields := strings.Fields(scanner.Text())
	if fields[0] != "cpu" {
		return 0, errors.New("invalid /proc/stat file")
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

// updateCPUCoresStats updates the provided serviceInfo cpuUsage and cpuTime stats.
func updateCPUCoresStats(pid int32, info *serviceInfo, lastGlobalCPUTime, currentGlobalCPUTime uint64) error {
	statPath := kernel.HostProc(strconv.Itoa(int(pid)), "stat")

	// This file is very small so just read it fully.
	content, err := os.ReadFile(statPath)
	if err != nil {
		return err
	}

	startIndex := bytes.LastIndexByte(content, byte(')'))
	if startIndex == -1 || startIndex+1 >= len(content) {
		return errors.New("invalid stat format")
	}

	// See proc(5) for a description of the format of statm and the fields.
	fields := strings.Fields(string(content[startIndex+1:]))
	if len(fields) < 50 {
		return errors.New("invalid stat format")
	}

	// Parse fields number 14 and 15, resp. User and System CPU time.
	// See proc_pid_stat(5), for details.
	// Here we address 11 & 12 since we skipped the first two fields.
	usrTime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return err
	}

	sysTime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return err
	}

	processTimeDelta := float64(usrTime + sysTime - info.cpuTime)
	globalTimeDelta := float64(currentGlobalCPUTime - lastGlobalCPUTime)

	info.cpuUsage = processTimeDelta / globalTimeDelta * float64(runtime.NumCPU())
	info.cpuTime = usrTime + sysTime

	return nil
}
