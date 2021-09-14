// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux_bpf

package probe

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const oomKilledPython = `
l = []
while True:
	l.append("." * (1024 * 1024))
`

const oomKilledBashScript = `
sysctl -w vm.overcommit_memory=1 # always overcommit
exec python3 %v # replace shell, so that the process launched by Go is the one getting oom-killed
`

func writeTempFile(pattern string, content string) (*os.File, error) {
	f, err := ioutil.TempFile("", pattern)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return nil, err
	}

	return f, nil
}

func TestOOMKillProbe(t *testing.T) {
	kv, err := kernel.HostVersion()
	if err != nil {
		t.Fatal(err)
	}
	if kv < kernel.VersionCode(4, 9, 0) {
		t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
	}

	cfg := ebpf.NewConfig()
	oomKillProbe, err := NewOOMKillProbe(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer oomKillProbe.Close()

	pf, err := writeTempFile("oom-kill-py", oomKilledPython)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(pf.Name())

	bf, err := writeTempFile("oom-trigger-sh", fmt.Sprintf(oomKilledBashScript, pf.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(bf.Name())

	cmd := exec.Command("bash", bf.Name())

	oomKilled := false
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				if (status.Signaled() && status.Signal() == unix.SIGKILL) || status.ExitStatus() == 137 {
					oomKilled = true
				}
			}
		}

		if !oomKilled {
			output, _ := cmd.CombinedOutput()
			t.Fatalf("expected process to be killed: %s (output: %s)", err, string(output))
		}
	}

	time.Sleep(3 * time.Second)

	found := false
	results := oomKillProbe.GetAndFlush()
	for _, result := range results {
		if result.TPid == uint32(cmd.Process.Pid) {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("failed to find an OOM killed process with pid %d in %+v", cmd.Process.Pid, results)
	}
}
