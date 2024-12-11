// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build freebsd || linux || netbsd || openbsd || solaris || dragonfly || aix

package pidfile

import (
	"os"
	"path/filepath"
	"strconv"
)

// isProcess searches for the PID under /proc
func isProcess(pid int) bool {
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	return err == nil
}

// Path returns a suitable location for the pidfile under Linux
func Path() string {
	return "/var/run/datadog/datadog-agent.pid"
}
