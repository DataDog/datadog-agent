// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package java

import (
	"io/ioutil"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/gopsutil/process"
	"github.com/stretchr/testify/require"
)

func findJustWait(t *testing.T) (retpid int) {
	fn := func(pid int) error {
		proc, err := process.NewProcess(int32(pid))
		if err != nil {
			return nil // ignore process that didn't exist anymore
		}

		name, err := proc.Name()
		if err == nil && name == "java" {
			cmdline, err := proc.Cmdline()
			if err == nil && strings.Contains(cmdline, "JustWait") {
				retpid = pid
			}
		}
		return nil
	}

	err := util.WithAllProcs(util.HostProc(), fn)
	if err != nil {
		t.Fatalf("%v\n", err)
	}
	return retpid
}

func testInject(t *testing.T, prefix string) {
	go func() {
		o, err := testutil.RunCommand(prefix + "java -cp testdata JustWait")
		if err != nil {
			t.Logf("%v\n", err)
		}
		t.Log(o)
	}()

	time.Sleep(200 * time.Millisecond)
	pid := findJustWait(t)
	require.NotEqual(t, pid, 0, "Can't find java JustWait process")
	t.Log(pid)

	defer func() {
		process, _ := os.FindProcess(pid)
		process.Signal(syscall.SIGKILL)
		time.Sleep(200 * time.Millisecond) // give a chance to the process to give his report/output
	}()

	tfile, err := ioutil.TempFile("", "TestAgentLoaded.agentmain.*")
	require.NoError(t, err)
	tfile.Close()
	os.Remove(tfile.Name())
	defer os.Remove(tfile.Name())

	// equivalent to jattach <pid> load instrument false testdata/TestAgentLoaded.jar=<tempfile>
	err = InjectAgent(pid, "testdata/TestAgentLoaded.jar", tfile.Name())
	require.NoError(t, err)

	// check if agent was loaded
	_, err = os.Stat(tfile.Name())
	require.NoError(t, err)

	t.Log("=== Test Success ===")
}

func TestInject(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	if err != nil {
		t.Skip("Can't detect kernel version on this platform")
	}
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)
	if pre410Kernel {
		t.Skip("Kernel < 4.1.0 are not supported as /proc/pid/status doesn't report NSpid")
	}

	for _, tname := range []string{"host", "namespace"} {
		t.Run(tname, func(t *testing.T) {
			p := ""
			if tname == "namespace" {
				p = "unshare -p --fork "
				_, err = testutil.RunCommand(p + "id")
				if err != nil {
					t.Skipf("unshare not supported on this platform %s", err)
				}
			}
			testInject(t, p)
		})
	}
}
