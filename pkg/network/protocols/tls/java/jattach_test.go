// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

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

	var pids []int
	var err error
	require.Eventually(t, func() bool {
		pids, err = javatestutil.FindProcessByCommandLine("java", "JustWait")
		return len(pids) == 1
	}, time.Second*5, time.Millisecond*100)
	require.NoError(t, err)

	t.Cleanup(func() {
		process, err := os.FindProcess(pids[0])
		if err != nil {
			return
		}
		_ = process.Signal(syscall.SIGKILL)
		_, _ = process.Wait()
	})

	tfile, err := os.CreateTemp("", "TestAgentLoaded.agentmain.*")
	require.NoError(t, err)
	require.NoError(t, tfile.Close())
	require.NoError(t, os.Remove(tfile.Name()))
	// equivalent to jattach <pid> load instrument false testdata/TestAgentLoaded.jar=<tempfile>
	require.NoError(t, InjectAgent(pids[0], "testdata/TestAgentLoaded.jar", "testfile="+tfile.Name()))
	require.Eventually(t, func() bool {
		_, err = os.Stat(tfile.Name())
		return err == nil
	}, time.Second*15, time.Millisecond*100)
}

// We test injection on a java hotspot running
//
//	o on the host
//	o in the container, _simulated_ by running java in his own PID namespace
func TestInject(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 14, 0) {
		t.Skip("Java TLS injection tests can run only on USM supported machines.")
	}

	javaVersion, err := testutil.RunCommand("java -version")
	require.NoErrorf(t, err, "java is not installed")
	t.Logf("java version %v", javaVersion)

	t.Run("host", func(t *testing.T) {
		testInject(t, "")
	})

	t.Run("PID namespace", func(t *testing.T) {
		p := "unshare -p --fork "
		_, err = testutil.RunCommand(p + "id")
		if err != nil {
			t.Skipf("unshare not supported on this platform %s", err)
		}

		// running the target process in a new PID namespace
		// and testing if the test/platform give enough permission to do that
		testInject(t, p)
	})
}
