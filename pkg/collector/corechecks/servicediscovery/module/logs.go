// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func getLogFiles(pid int32, candidates []fdPath) []string {

	logs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		fdInfoPath := kernel.HostProc(fmt.Sprintf("%d/fdinfo/%s", pid, candidate.fd))
		fdInfo, err := os.ReadFile(fdInfoPath)
		if err != nil {
			continue
		}

		lines := strings.SplitN(string(fdInfo), "\n", 2)
		if len(lines) < 2 || !strings.HasPrefix(lines[1], "flags:") {
			continue
		}
		fields := strings.Fields(lines[1])
		if len(fields) < 2 {
			continue
		}
		flags, err := strconv.ParseUint(fields[1], 8, 32)
		if err != nil {
			continue
		}
		// O_WRONLY is 1
		if flags&1 == 1 {
			logs = append(logs, candidate.path)
		}
	}
	return logs
}
