// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package probe

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const oomKilledPython = `
l = []
while True:
	l.append("." * (1024 * 1024))
`

const oomKilledBashScript = `
exec systemd-run --scope -p MemoryLimit=1M python3 %v # replace shell, so that the process launched by Go is the one getting oom-killed
`

func writeTempFile(pattern string, content string) (*os.File, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return nil, err
	}

	return f, nil
}

func TestOOMKillCompile(t *testing.T) {
	kv, err := kernel.HostVersion()
	if err != nil {
		t.Fatal(err)
	}
	if kv < kernel.VersionCode(4, 9, 0) {
		t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
	}

	cfg := testConfig()
	cfg.BPFDebug = true
	_, err = runtime.OomKill.Compile(cfg, []string{"-g"}, statsd.Client)
	require.NoError(t, err)
}

func TestOOMKillProbe(t *testing.T) {
	kv, err := kernel.HostVersion()
	if err != nil {
		t.Fatal(err)
	}
	if kv < kernel.VersionCode(4, 9, 0) {
		t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
	}

	cfg := testConfig()

	fullKV := host.GetStatusInformation().KernelVersion
	if cfg.EnableCORE && (fullKV == "4.18.0-1018-azure" || fullKV == "4.18.0-147.43.1.el8_1.x86_64") {
		t.Skipf("Skipping CO-RE tests for kernel version %v due to missing BTFs", fullKV)
	}

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

			assert.Regexp(t, regexp.MustCompile("run-([0-9|a-z]*).scope"), result.CgroupName, "cgroup name")
			assert.Equal(t, result.TPid, result.Pid, "tpid == pid")
			assert.Equal(t, "python3", result.FComm, "fcomm")
			assert.Equal(t, "python3", result.TComm, "tcomm")
			assert.NotZero(t, result.Pages, "pages")
			assert.Equal(t, uint32(1), result.MemCgOOM, "memcg oom")
			break
		}
	}

	if !found {
		t.Errorf("failed to find an OOM killed process with pid %d in %+v", cmd.Process.Pid, results)
	}
}

func testConfig() *ebpf.Config {
	cfg := ebpf.NewConfig()
	return cfg
}
