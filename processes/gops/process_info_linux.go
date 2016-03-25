package gops

import (
	"fmt"

	"github.com/shirou/gopsutil/process"
)

// Build a hash pid -> ppid
func buildPIDHash(processInfos []*ProcessInfo) (hash map[int32]int32) {
	hash = make(map[int32]int32)
	for _, processInfo := range processInfos {
		hash[processInfo.PID] = processInfo.PPID
	}
	return
}

// Return whether the PID is of a kernel thread, based on whether it has
// the init process (PID 1) as ancestor
func isKernelThread(pid int32, pidHash map[int32]int32) bool {
	if pid == 1 {
		return false
	}
	ppid, ok := pidHash[pid]
	if !ok {
		return true
	}

	return isKernelThread(ppid, pidHash)
}

// Name processes "kernel" if they're a kernel thread
func postProcess(processInfos []*ProcessInfo) {
	pidHash := buildPIDHash(processInfos)
	for _, processInfo := range processInfos {
		if isKernelThread(processInfo.PID, pidHash) {
			processInfo.Name = "kernel"
		}
	}
}
