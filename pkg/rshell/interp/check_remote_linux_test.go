// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package interp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
	"github.com/stretchr/testify/require"
)

// A syscall kill filter detects attempted I/O even when the caller would ignore
// its error. It is installed before the first CheckRemote (including builtin
// registration), across every thread. The child exits without testing's output.
func TestCheckRemoteNoHostIO(t *testing.T) {
	const helperEnv = "RSHELL_REMOTE_NO_IO_HELPER"
	if mode := os.Getenv(helperEnv); mode != "" {
		runRemoteNoIOHelper(mode)
		os.Exit(0)
	}
	for _, mode := range []string{"check", "timer-wakeup", "stat-canary", "read-canary", "write-canary"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCheckRemoteNoHostIO$", "-test.timeout=5m")
			cmd.Env = append(os.Environ(), helperEnv+"="+mode)
			output, err := cmd.CombinedOutput()
			if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 77 {
				t.Skipf("seccomp unavailable: %s", output)
			}
			if mode == "check" || mode == "timer-wakeup" {
				require.NoError(t, err, "remote check attempted forbidden host I/O: %s", output)
			} else {
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, "filter canary must be killed")
				require.Equal(t, -1, exit.ExitCode(), "canary must die from a signal, not a normal exit: %s", output)
				require.NoError(t, ctx.Err(), "canary must not have timed out")
			}
		})
	}
}

func runRemoteNoIOHelper(mode string) {
	if !seccomp.Supported() {
		os.Exit(77)
	}
	info, err := arch.GetInfo(runtime.GOARCH)
	if err != nil {
		os.Exit(77)
	}
	// Initialize the poller before identifying its eventfd. Runtime timer wakeups
	// use eight-byte reads/writes there, including with Bazel's test timeout.
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	time.Sleep(time.Millisecond)
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var eventFDs seccomp.ArgumentConditions
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if target != "anon_inode:[eventfd]" {
			continue
		}
		fd, err := strconv.ParseUint(entry.Name(), 10, 64)
		if err != nil {
			os.Exit(1)
		}
		eventFDs = append(eventFDs, seccomp.Condition{Argument: 0, Operation: seccomp.NotEqual, Value: fd})
	}
	if len(eventFDs) == 0 {
		fmt.Fprintln(os.Stderr, "runtime eventfd not found")
		os.Exit(1)
	}
	// Keep each argument-filtered syscall in its own filter: the assembler does
	// not restore the syscall-number accumulator after inspecting arguments.
	for _, name := range []string{"read", "write"} {
		if err := seccomp.LoadFilter(seccomp.Filter{
			NoNewPrivs: true, Flag: seccomp.FilterFlagTSync,
			Policy: seccomp.Policy{DefaultAction: seccomp.ActionAllow, Syscalls: []seccomp.SyscallGroup{{
				Action: seccomp.ActionKillProcess,
				NamesWithCondtions: []seccomp.NameWithConditions{
					{Name: name, Conditions: eventFDs},
					{Name: name, Conditions: seccomp.ArgumentConditions{{Argument: 2, Operation: seccomp.NotEqual, Value: 8}}},
				},
			}}},
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	group := seccomp.SyscallGroup{Action: seccomp.ActionKillProcess}
	for _, name := range []string{
		"open", "openat", "openat2", "creat", "stat", "lstat", "fstat", "statx", "newfstatat", "fstatat", "fstatat64",
		"statfs", "fstatfs", "access", "faccessat", "faccessat2", "getcwd", "chdir", "fchdir",
		"readlink", "readlinkat", "getdents", "getdents64", "readv", "pread64", "preadv", "preadv2",
		"writev", "pwrite64", "pwritev", "pwritev2", "truncate", "ftruncate", "unlink", "unlinkat",
		"rename", "renameat", "renameat2", "mkdir", "mkdirat", "rmdir", "chmod", "fchmod", "fchmodat",
		"link", "linkat", "symlink", "symlinkat", "execve", "execveat", "fork", "vfork",
		"socket", "connect", "sendto", "recvfrom", "sendmsg", "recvmsg", "setuid", "setgid", "setresuid", "setresgid",
	} {
		if _, ok := info.SyscallNames[name]; ok {
			group.Names = append(group.Names, name)
		}
	}
	if err := seccomp.LoadFilter(seccomp.Filter{
		NoNewPrivs: true, Flag: seccomp.FilterFlagTSync,
		Policy: seccomp.Policy{DefaultAction: seccomp.ActionAllow, Syscalls: []seccomp.SyscallGroup{group}},
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	switch mode {
	case "timer-wakeup":
		timer.Reset(time.Millisecond)
		<-timer.C
		return
	case "stat-canary":
		_, _ = os.Stat("/")
		return
	case "read-canary":
		_, _ = os.Stdin.Read(make([]byte, 8))
		return
	case "write-canary":
		_, _ = os.Stdout.Write([]byte("must not"))
		return
	}
	policy := RemotePolicy{
		AllowedCommands: []string{"rshell:cat", "rshell:echo", "rshell:stat", "rshell:ls", "rshell:rm", "rshell:truncate", "rshell:read", "rshell:df", "rshell:free", "rshell:ss", "rshell:ps", "rshell:systemctl", "rshell:journalctl"},
		AllowedPaths:    []string{"/rshell-remote-not-local:rw"}, Mode: ModeRemediation,
		ElevatableCommands:    []string{"rshell:echo", "rshell:systemctl"},
		AllowedSystemServices: []SystemServiceControlGrant{{Service: "app.service", Actions: []SystemServiceAction{SystemServiceAllActions}}},
	}
	for i, test := range []struct {
		script string
		status CheckStatus
	}{
		{"echo hi", CheckAllowed},
		{"cat /rshell-remote-not-local/file", CheckIndeterminate},
		{"echo hi > /rshell-remote-not-local/file", CheckIndeterminate},
		{"stat /rshell-remote-not-local/file; ls /rshell-remote-not-local", CheckIndeterminate},
		{"rm /rshell-remote-not-local/file; truncate -s 0 /rshell-remote-not-local/file", CheckIndeterminate},
		{"cat /etc/passwd", CheckDenied},
		{"cat; read VALUE", CheckAllowed},
		{"echo \"$(cat /rshell-remote-not-local/file)\"", CheckIndeterminate},
		{"cat /rshell-remote-not-local/*", CheckIndeterminate},
		{"df; free; ss; ps", CheckAllowed},
		{"sudo echo hi; sudo systemctl restart app.service; journalctl -u app.service", CheckAllowed},
		{"cat ~root/file", CheckDenied},
	} {
		result, err := CheckRemote(context.Background(), test.script, policy)
		if err != nil || result == nil || result.Status != test.status || result.Allowed != (test.status == CheckAllowed) {
			os.Exit(10 + i)
		}
	}
}
