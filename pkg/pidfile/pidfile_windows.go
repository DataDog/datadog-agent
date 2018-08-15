// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package pidfile

import "syscall"

const (
	processQueryLimitedInformation = 0x1000

	stillActive = 259
)

// isProcess checks to see if a given pid is currently valid in the process table
func isProcess(pid int) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	var c uint32
	err = syscall.GetExitCodeProcess(h, &c)
	syscall.Close(h)
	if err != nil {
		return c == stillActive
	}
	return true
}

// Path returns a suitable location for the pidfile under Windows
func Path() string {
	return filepath.Join(os.Getenv("ProgramData"), "DataDog", "datadog-agent.pid")
}
