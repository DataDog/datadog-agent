// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package testutil

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IptablesSave saves the current iptables state to a file
// and returns its path
func IptablesSave(t *testing.T) string {
	cmd := exec.Command("iptables-save")
	f, err := os.CreateTemp(t.TempDir(), "iptables-save-")
	require.NoError(t, err)
	cmd.Stdout = f
	err = cmd.Run()
	require.NoError(t, err)

	return f.Name()
}

// IptablesRestore restores iptables state from a file
func IptablesRestore(t *testing.T, path string) {
	cmd := exec.Command("iptables-restore")
	f, err := os.Open(path)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	cmd.Stdin = f
	assert.NoError(t, cmd.Run())
}

// Ip6tablesSave saves the current iptables state to a file
// and returns its path
func Ip6tablesSave(t *testing.T) string {
	cmd := exec.Command("ip6tables-save")
	f, err := os.CreateTemp(t.TempDir(), "iptables-save-")
	require.NoError(t, err)
	cmd.Stdout = f
	err = cmd.Run()
	require.NoError(t, err)

	return f.Name()
}

// Ip6tablesRestore restores iptables state from a file
func Ip6tablesRestore(t *testing.T, path string) {
	cmd := exec.Command("ip6tables-restore")
	f, err := os.Open(path)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	cmd.Stdin = f
	assert.NoError(t, cmd.Run())
}
