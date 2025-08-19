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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const (
	poolCapacity = 100
)

// ignoreFamily list of processes with hyphens in their names,
// matching up to the hyphen excludes process from reporting.
var ignoreFamily = map[string]struct{}{
	"systemd":    {}, // 'systemd-networkd', 'systemd-resolved' etc
	"datadog":    {}, // datadog processes
	"containerd": {}, // 'containerd-shim...'
	"docker":     {}, // 'docker-proxy'
}

var (
	procCommBufferPool = ddsync.NewSlicePool[byte](core.MaxCommLen, poolCapacity)
)

// shouldIgnoreComm returns true if process should be ignored
func (s *discovery) shouldIgnoreComm(pid int32) bool {
	if s.config.IgnoreComms == nil {
		return false
	}
	commPath := kernel.HostProc(strconv.Itoa(int(pid)), "comm")
	file, err := os.Open(commPath)
	if err != nil {
		return true
	}
	defer file.Close()

	buf := procCommBufferPool.Get()
	defer procCommBufferPool.Put(buf)

	n, err := file.Read(*buf)
	if err != nil {
		return true
	}
	dash := bytes.IndexByte((*buf)[:n], '-')
	if dash > 0 {
		_, found := ignoreFamily[string((*buf)[:dash])]
		if found {
			return true
		}
	}

	comm := strings.TrimSuffix(string((*buf)[:n]), "\n")
	_, found := s.config.IgnoreComms[comm]

	return found
}
