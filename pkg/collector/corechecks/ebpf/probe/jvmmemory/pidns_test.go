// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestPidNamespaceConstant(t *testing.T) {
	procRoot := t.TempDir()
	_, err := pidNamespaceConstant(procRoot)
	assert.Error(t, err, "no /proc/1/ns/pid")

	var self unix.Stat_t
	if err := unix.Stat("/proc/self/ns/pid", &self); err != nil {
		t.Skipf("PID namespace not readable: %s", err)
	}
	selfIno := self.Ino

	// a procfs whose init process is in the PID namespace of the test
	require.NoError(t, os.MkdirAll(procPidPath(procRoot, 1, "ns"), 0o755))
	require.NoError(t, os.Symlink("/proc/self/ns/pid", procPidPath(procRoot, 1, "ns", "pid")))
	ino, err := pidNamespaceConstant(procRoot)
	require.NoError(t, err)
	if selfIno == procPidInitIno {
		assert.Zero(t, ino, "root PID namespace")
	} else {
		assert.Equal(t, selfIno, ino)
	}
}
