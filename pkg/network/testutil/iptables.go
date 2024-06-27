// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package testutil

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IptablesSave saves the current iptables state to a file
// and returns its path
func IptablesSave(tb testing.TB) []byte {
	cmd := exec.Command("iptables-save")
	state, err := cmd.Output()
	require.NoError(tb, err)

	// make sure the nat table is saved,
	// on some machines on startup, with the
	// nat table empty, iptables-save will not
	// include the nat table, so that restoring
	// from this state will not remove any nat
	// rules added by tests
	cmd = exec.Command("iptables-save", "-t", "nat")
	natState, err := cmd.Output()
	require.NoError(tb, err)
	fullState := append(state, natState...)
	tb.Cleanup(func() {
		IptablesRestore(tb, fullState)
	})
	return fullState
}

// IptablesRestore restores iptables state from a file
func IptablesRestore(tb testing.TB, state []byte) {
	var restoreErr error
	// The attempt mechanism is necessary because we noticed that iptables-restore fails with error code 4 when it can't acquire the lock.
	// We can't rely on the --wait flag as it's not available on older versions of iptables.
	for attempt := 0; attempt < 3; attempt++ {
		cmd := exec.Command("iptables-restore", "--counters")
		cmd.Stdin = bytes.NewReader(state)
		restoreErr = cmd.Run()

		// If no error occurs, return early.
		if restoreErr == nil {
			return
		}
	}
	assert.NoError(tb, restoreErr)
}

// Ip6tablesSave saves the current iptables state to a file
// and returns its path
//
//nolint:revive // TODO(NET) Fix revive linter
func Ip6tablesSave(tb testing.TB) {
	cmd := exec.Command("ip6tables-save")
	state, err := cmd.Output()
	require.NoError(tb, err)

	// make sure the nat table is saved,
	// on some machines on startup, with the
	// nat table empty, iptables-save will not
	// include the nat table, so that restoring
	// from this state will not remove any nat
	// rules added by tests
	cmd = exec.Command("ip6tables-save", "-t", "nat")
	natState, err := cmd.Output()
	require.NoError(tb, err)
	fullState := append(state, natState...)
	tb.Cleanup(func() {
		Ip6tablesRestore(tb, fullState)
	})
}

// Ip6tablesRestore restores iptables state from a file
//
//nolint:revive // TODO(NET) Fix revive linter
func Ip6tablesRestore(tb testing.TB, state []byte) {
	cmd := exec.Command("ip6tables-restore", "--counters")
	cmd.Stdin = bytes.NewReader(state)
	assert.NoError(tb, cmd.Run())
}
