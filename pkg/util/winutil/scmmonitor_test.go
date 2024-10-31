// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package winutil

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

type saveOutput struct {
	savedOutput []byte
}

func (so *saveOutput) Write(p []byte) (n int, err error) {
	so.savedOutput = append(so.savedOutput, p...)
	return os.Stdout.Write(p)
}

var startTime uint64
var startTimeOnce sync.Once

func testGetProcessStartTimeAsNs(_ uint64) (uint64, error) {

	startTimeOnce.Do(func() {
		var t windows.Filetime
		windows.GetSystemTimeAsFileTime(&t)
		// move the start a bit earlier to make sure it's always before current.
		// 16ms is Windows time resolution, so use a few of those.
		offset := (3 * 16 * time.Millisecond).Nanoseconds()
		ns := t.Nanoseconds()
		if ns < offset {
			panic("time is too early")
		}
		startTime = uint64(ns - offset)
	})
	return startTime, nil

}
func TestServices(t *testing.T) {
	var output saveOutput
	cmd := exec.Command("tasklist", "/svc", "/fo", "csv")
	cmd.Stdin = os.Stdin
	cmd.Stdout = &output
	cmd.Stderr = os.Stderr
	_ = cmd.Run()

	pGetProcessStartTimeAsNs = testGetProcessStartTimeAsNs
	scm := GetServiceMonitor()

	for _, line := range strings.Split(strings.TrimRight(string(output.savedOutput), "\r\n"), "\r\n")[1:] {

		entries := strings.Split(line, ",")
		for i, s := range entries {
			entries[i] = strings.Replace(s, "\"", "", -1)
		}
		pid, _ := strconv.ParseInt(entries[1], 10, 64)
		if pid == 0 {
			continue
		}
		if pid == int64(windows.GetCurrentProcessId()) {
			continue
		}
		if pid == int64(cmd.Process.Pid) {
			continue
		}
		si, err := scm.GetServiceInfo(uint64(pid))
		assert.Nil(t, err, "Error on pid %v", pid)
		if entries[2] != "N/A" {
			if assert.NotNil(t, si) {
				for _, name := range entries[2:] {
					assert.Contains(t, si.ServiceName, name)
				}
			} else {
				t.Logf("Unexpected empty entry %v", entries)
			}
		} else {
			// the "N/A" processes are not services, so should not get
			// an entry back
			assert.Nil(t, si)
		}
	}
	count := scm.GetRefreshCount()
	assert.EqualValues(t, 1, count)

}
