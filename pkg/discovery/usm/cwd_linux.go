// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func getWorkingDirectoryFromPid(pid int) (string, bool) {
	cwdPath := kernel.HostProc(strconv.Itoa(pid), "cwd")
	if target, err := os.Readlink(cwdPath); err == nil {
		return target, true
	}
	return "", false
}
