// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
)

func TestProcToMsgBoundsArgs(t *testing.T) {
	largeArgs := make([]string, sharedconsts.MaxArgsEnvsSize+1)
	for i := range largeArgs {
		largeArgs[i] = strings.Repeat("a", 4*sharedconsts.MaxArgEnvSize)
	}
	for _, tc := range []struct {
		name      string
		args      []string
		truncated bool
	}{
		{name: "small argv", args: []string{"--worker"}},
		{name: "oversized argv", args: largeArgs, truncated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Block in a shell builtin until cleanup: no sleeps, polling or
			// extra child processes. The large argv is legal on Linux but
			// exceeds the wire limit before truncation.
			args := append([]string{"-c", "read -r line", "snapshot"}, tc.args...)
			cmd := exec.Command("/bin/sh", args...)
			cmd.Env = []string{"PATH=/bin:/usr/bin"}
			stdin, err := cmd.StdinPipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = stdin.Close() })
			require.NoError(t, cmd.Start())
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			})

			proc, err := collectProcess(int32(cmd.Process.Pid))
			require.NoError(t, err)
			message, err := procToMsg(proc)
			require.NoError(t, err)
			require.NotNil(t, message.Syscall)
			image := message.Syscall.Exec
			require.NotNil(t, image)
			assert.Equal(t, ebpfless.SyscallTypeExec, message.Syscall.Type)
			assert.True(t, image.FromProcFS)
			assert.EqualValues(t, cmd.Process.Pid, message.Syscall.PID)
			assert.EqualValues(t, os.Getpid(), image.PPID)
			assert.NotEmpty(t, image.File.Filename)
			assert.Equal(t, tc.truncated, image.ArgsTruncated)
			assert.LessOrEqual(t, len(image.Args), sharedconsts.MaxArgsEnvsSize)
			for _, arg := range image.Args {
				assert.LessOrEqual(t, len(arg), sharedconsts.MaxArgEnvSize)
			}
			if !tc.truncated {
				assert.Equal(t, cmd.Args, image.Args)
			} else {
				require.Len(t, image.Args, sharedconsts.MaxArgsEnvsSize)
				assert.Equal(t, strings.Repeat("a", sharedconsts.MaxArgEnvSize-4)+"...", image.Args[4])
			}

			data, err := msgpack.Marshal(message)
			require.NoError(t, err)
			assert.LessOrEqual(t, len(data), ebpfless.MaxMessageSize)
		})
	}
}
