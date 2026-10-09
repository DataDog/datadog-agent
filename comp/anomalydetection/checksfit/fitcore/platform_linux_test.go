// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

//go:build linux

package fitcore

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestShmOpenRejectsSymlinkedNames verifies the O_NOFOLLOW hardening: a
// symlink planted at a POSIX shm name must fail with ELOOP instead of
// redirecting the producer's open, matching glibc's shm_open.
func TestShmOpenRejectsSymlinkedNames(t *testing.T) {
	link := filepath.Join("/dev/shm", "mc-symlink-probe")
	if _, err := os.Stat("/dev/shm"); err != nil {
		t.Skip("/dev/shm is unavailable")
	}
	if err := os.Symlink("/dev/shm/mc-unrelated", link); err != nil {
		if errors.Is(err, os.ErrExist) {
			os.Remove(link)
		} else {
			t.Skipf("cannot create probe symlink: %v", err)
		}
	}
	defer os.Remove(link)
	if _, err := shmOpen("/mc-symlink-probe", syscall.O_RDWR, 0); !errors.Is(err, syscall.ELOOP) {
		t.Fatalf("symlinked shm name opened with error %v, want ELOOP", err)
	}
}
