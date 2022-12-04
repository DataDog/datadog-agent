// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

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
	return append(state, natState...)
}

// IptablesRestore restores iptables state from a file
func IptablesRestore(tb testing.TB, state []byte) {
	cmd := exec.Command("iptables-restore", "--counters")
	cmd.Stdin = bytes.NewReader(state)
	assert.NoError(tb, cmd.Run())
}

// Ip6tablesSave saves the current iptables state to a file
// and returns its path
func Ip6tablesSave(t *testing.T) []byte {
	cmd := exec.Command("ip6tables-save")
	state, err := cmd.Output()
	require.NoError(t, err)

	// make sure the nat table is saved,
	// on some machines on startup, with the
	// nat table empty, iptables-save will not
	// include the nat table, so that restoring
	// from this state will not remove any nat
	// rules added by tests
	cmd = exec.Command("ip6tables-save", "-t", "nat")
	natState, err := cmd.Output()
	require.NoError(t, err)
	return append(state, natState...)
}

// Ip6tablesRestore restores iptables state from a file
func Ip6tablesRestore(t *testing.T, state []byte) {
	cmd := exec.Command("ip6tables-restore", "--counters")
	cmd.Stdin = bytes.NewReader(state)
	assert.NoError(t, cmd.Run())
}
