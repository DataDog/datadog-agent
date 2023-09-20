// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package oomkill

import (
	"context"
	"os/exec"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/oomkill/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var kv = kernel.MustHostVersion()

func TestOOMKillCompile(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		if kv < kernel.VersionCode(4, 9, 0) {
			t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
		}

		cfg := testConfig()
		cfg.BPFDebug = true
		out, err := runtime.OomKill.Compile(cfg, []string{"-g"}, statsd.Client)
		require.NoError(t, err)
		_ = out.Close()
	})
}

func TestOOMKillProbe(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		if kv < kernel.VersionCode(4, 9, 0) {
			t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
		}

		cfg := testConfig()
		oomKillProbe, err := NewProbe(cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(oomKillProbe.Close)

		t.Cleanup(func() {
			out, err := exec.Command("swapon", "-a").CombinedOutput()
			if err != nil {
				t.Logf("swapon -a: %s: %s", err, out)
			}
		})
		require.NoError(t, exec.Command("swapoff", "-a").Run())

		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		t.Cleanup(cancel)

		cmd := exec.CommandContext(ctx, "systemd-run", "--scope", "-p", "MemoryLimit=1M", "dd", "if=/dev/zero", "of=/dev/shm/asdf", "bs=1K", "count=2K")
		obytes, err := cmd.CombinedOutput()
		output := string(obytes)
		require.Error(t, err)
		require.NotErrorIs(t, err, context.DeadlineExceeded)

		var exiterr *exec.ExitError
		require.ErrorAs(t, err, &exiterr, output)
		var status syscall.WaitStatus

		status, sok := exiterr.Sys().(syscall.WaitStatus)
		require.True(t, sok, output)

		if status.Signaled() {
			require.Equal(t, unix.SIGKILL, status.Signal(), output)
		} else {
			require.Equal(t, 128+unix.SIGKILL, status.ExitStatus(), output)
		}

		var result model.OOMKillStats
		require.Eventually(t, func() bool {
			for _, r := range oomKillProbe.GetAndFlush() {
				if r.TPid == uint32(cmd.Process.Pid) {
					result = r
					return true
				}
			}
			return false
		}, 5*time.Second, 500*time.Millisecond, "failed to find an OOM killed process with pid %d", cmd.Process.Pid)

		assert.Regexp(t, regexp.MustCompile("run-([0-9|a-z]*).scope"), result.CgroupName, "cgroup name")
		assert.Equal(t, result.TPid, result.Pid, "tpid == pid")
		assert.Equal(t, "dd", result.FComm, "fcomm")
		assert.Equal(t, "dd", result.TComm, "tcomm")
		assert.NotZero(t, result.Pages, "pages")
		assert.Equal(t, uint32(1), result.MemCgOOM, "memcg oom")
	})
}

func testConfig() *ebpf.Config {
	cfg := ebpf.NewConfig()
	return cfg
}
