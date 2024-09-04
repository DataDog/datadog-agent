// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"errors"
	"os"
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
