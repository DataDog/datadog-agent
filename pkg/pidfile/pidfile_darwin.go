// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pidfile

import (
	"syscall"
)

// isProcess uses `kill -0` to check whether a process is running
func isProcess(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// Path returns a suitable location for the pidfile under OSX
func Path() string {
	return "/var/run/datadog/datadog-agent.pid"
}
