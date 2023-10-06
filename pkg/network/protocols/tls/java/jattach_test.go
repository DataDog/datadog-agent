// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package java

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	javatestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/java/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func testInject(t *testing.T, prefix string) {
	go func() {
		o, err := testutil.RunCommand(prefix + "java -cp testdata Wait JustWait")
		if err != nil {
			t.Logf("%v\n", err)
		}
		t.Log(o)
	}()
	time.Sleep(time.Second) // give a chance to spawn java

	pids, err := javatestutil.FindProcessByCommandLine("java", "JustWait")
	require.NoError(t, err)
	require.Lenf(t, pids, 1, "expected to find 1 match, but found %v instead", len(pids))

	defer func() {
		process, _ := os.FindProcess(pids[0])
		process.Signal(syscall.SIGKILL)
		time.Sleep(200 * time.Millisecond) // give a chance to the process to give his report/output
	}()

	tfile, err := os.CreateTemp("", "TestAgentLoaded.agentmain.*")
	require.NoError(t, err)
	tfile.Close()
	os.Remove(tfile.Name())
	defer os.Remove(tfile.Name())

	// equivalent to jattach <pid> load instrument false testdata/TestAgentLoaded.jar=<tempfile>
	err = InjectAgent(pids[0], "testdata/TestAgentLoaded.jar", "testfile="+tfile.Name())
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
