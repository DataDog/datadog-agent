// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package uname_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// Linux only provides /proc/sys/kernel/arch since 6.1 (e.g. RHEL 9's 5.14
// kernel lacks it); -m must then fall back to uname(2) rather than fail.
func TestUnameMachineFallsBackWithoutArchFile(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	vals := defaultFakeProc()
	delete(vals, "arch")
	writeFakeProc(t, dir, vals)

	var uts unix.Utsname
	require.NoError(t, unix.Uname(&uts))
	machine := unix.ByteSliceToString(uts.Machine[:])
	require.NotEmpty(t, machine)

	stdout, stderr, code := cmdRun(t, "uname -m; uname -a", dir)
	assert.Equal(t, 0, code, stderr)
	assert.Equal(t, machine+"\nLinux testhost 5.15.0-test #1 SMP Test "+machine+"\n", stdout)
}
