// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package seccomp

import (
	"slices"
	"testing"
)

func TestDefaultDenylistReturnsCopy(t *testing.T) {
	first := DefaultDenylist()
	second := DefaultDenylist()
	if len(first) == 0 {
		t.Fatal("default denylist must not be empty")
	}

	first[0] = "changed"
	if second[0] == "changed" {
		t.Fatal("DefaultDenylist returned shared mutable storage")
	}
}

func TestDefaultDenylistReviewedExceptions(t *testing.T) {
	denied := DefaultDenylist()

	for _, name := range []string{
		"clone", "clone3", "fork", "vfork", "execve", "execveat",
		"setuid", "setgid", "setgroups", "capset", "prctl",
		"unshare", "setns", "mount", "umount2", "pivot_root",
		"bpf", "perf_event_open", "ptrace", "process_vm_readv", "process_vm_writev",
		"keyctl", "add_key", "request_key",
		"init_module", "finit_module", "delete_module",
		"reboot", "mknod", "mknodat", "ioctl",
		"chmod", "fchmodat2", "chown", "setxattr", "removexattr", "utimensat",
		"kill", "settimeofday", "clock_settime", "setpriority", "sched_setattr",
		"io_uring_setup", "userfaultfd", "open_by_handle_at",
	} {
		if !slices.Contains(denied, name) {
			t.Errorf("default denylist is missing %q", name)
		}
	}

	for _, name := range []string{"setresuid"} {
		if slices.Contains(denied, name) {
			t.Errorf("worker-required syscall %q must not be denied", name)
		}
	}
}

func TestDefaultDenylistHasNoDuplicates(t *testing.T) {
	seen := make(map[string]struct{})
	for _, name := range DefaultDenylist() {
		if _, exists := seen[name]; exists {
			t.Fatalf("duplicate syscall %q", name)
		}
		seen[name] = struct{}{}
	}
}
