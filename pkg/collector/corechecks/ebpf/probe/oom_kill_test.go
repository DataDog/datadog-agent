// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package probe

import (
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
		oomKillProbe, err := NewOOMKillProbe(cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(oomKillProbe.Close)

		cmd := exec.Command("systemd-run", "--scope", "-p", "MemoryLimit=1M", "dd", "if=/dev/zero", "of=/dev/null", "bs=2M")
		err = cmd.Start()
		require.NoError(t, err)

		to := 3 * time.Minute
		manuallyKilled := false
		done := make(chan struct{})
		go func() {
			select {
			case <-time.After(to):
				manuallyKilled = true
				_ = cmd.Process.Kill()
				return
			case <-done:
				return
			}
		}()

		oomKilled := false
		err = cmd.Wait()
		close(done)
		require.False(t, manuallyKilled, "process timed out after %s", to)

		if err != nil {
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
				assert.Equal(t, "dd", result.FComm, "fcomm")
				assert.Equal(t, "dd", result.TComm, "tcomm")
				assert.NotZero(t, result.Pages, "pages")
				assert.Equal(t, uint32(1), result.MemCgOOM, "memcg oom")
				break
			}
		}

		if !found {
			t.Errorf("failed to find an OOM killed process with pid %d in %+v", cmd.Process.Pid, results)
		}
	})
}

func testConfig() *ebpf.Config {
	cfg := ebpf.NewConfig()
	return cfg
}
