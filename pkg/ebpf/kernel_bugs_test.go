// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasTasksRCUExitLockSymbol(t *testing.T) {
	funcCache = newExistCache("./testdata/kallsyms.fentry.bug")

	hasDeadlock, err := HasTasksRCUExitLockSymbol()
	require.NoError(t, err)
	require.True(t, hasDeadlock)

	funcCache = newExistCache("./testdata/kallsyms.fentry.nobug")
	hasDeadlock, err = HasTasksRCUExitLockSymbol()
	require.NoError(t, err)
	require.False(t, hasDeadlock)
}
