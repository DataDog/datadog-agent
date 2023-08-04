// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

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
	time.Sleep(time.Second) // give a chance to spawn java

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
	err = InjectAgent(pid, "testdata/TestAgentLoaded.jar", "testfile="+tfile.Name())
	require.NoError(t, err)

	time.Sleep((MINIMUM_JAVA_AGE_TO_ATTACH_MS + 200) * time.Millisecond) // wait java process to be old enough to be injected

	// check if agent was loaded
	_, err = os.Stat(tfile.Name())
	require.NoError(t, err)

	t.Log("=== Test Success ===")
}

// We test injection on a java hotspot running
//
//	o on the host
//	o in the container, _simulated_ by running java in his own PID namespace
func TestInject(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	if err != nil {
		t.Skip("Can't detect kernel version on this platform")
	}
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)
	if pre410Kernel {
		t.Skip("Kernel < 4.1.0 are not supported as /proc/pid/status doesn't report NSpid")
	}

	javaVersion, err := testutil.RunCommand("java -version")
	if err != nil {
		t.Fatal("java is not installed", javaVersion, err)
	}
	t.Log(javaVersion)

	t.Run("host", func(t *testing.T) {
		testInject(t, "")
	})
	if t.Failed() {
		t.Fatal("host failed")
	}

	t.Run("PIDnamespace", func(t *testing.T) {
		p := "unshare -p --fork "
		_, err = testutil.RunCommand(p + "id")
		if err != nil {
			t.Skipf("unshare not supported on this platform %s", err)
		}

		// running the tagert process in a new PID namespace
		// and testing if the test/plaform give enough permission to do that
		testInject(t, p)
	})

}
