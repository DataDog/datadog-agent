// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"bytes"
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	// maximum command name length to process when checking for non-reportable commands,
	// is one byte less (excludes end of line) than the maximum of /proc/<pid>/comm
	// defined in https://man7.org/linux/man-pages/man5/proc.5.html.
	maxCommLen = 15
)

// ignoreFamily list of processes with hyphens in their names,
// matching up to the hyphen excludes process from reporting.
var ignoreFamily = map[string]struct{}{
	"systemd":    {}, // 'systemd-networkd', 'systemd-resolved' etc
	"datadog":    {}, // datadog processes
	"containerd": {}, // 'containerd-shim...'
	"docker":     {}, // 'docker-proxy'
}

// shouldIgnoreComm returns true if process should be ignored
func (s *discovery) shouldIgnoreComm(proc *process.Process) bool {
	if s.config.ignoreComms == nil {
		return false
	}
	commPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "comm")
	contents, err := os.ReadFile(commPath)
	if err != nil {
		return true
	}

	dash := bytes.IndexByte(contents, '-')
	if dash > 0 {
		_, found := ignoreFamily[string(contents[:dash])]
		if found {
			return true
		}
	}

	comm := strings.TrimSuffix(string(contents), "\n")
	_, found := s.config.ignoreComms[comm]

	return found
}
