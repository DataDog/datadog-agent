// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux_bpf

package probe

import (
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

const oomKilledPython = `
l = []
while True:
	l.append("." * (1024 * 1024))
`

func TestOOMKillProbe(t *testing.T) {
	cfg := ebpf.NewConfig()
	oomKillProbe, err := NewOOMKillProbe(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer oomKillProbe.Close()

	f, err := ioutil.TempFile("", "oom-kill-py")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.WriteString(oomKilledPython); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	cmd := exec.Command("python3", f.Name())

	oomKilled := false
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				if status.Signaled() && status.Signal() == unix.SIGKILL {
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
