// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func TestAllowedCommandsNamespaceRequired(t *testing.T) {
	_, err := interp.New(interp.AllowedCommands([]string{"echo"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing namespace prefix")
}

func TestAllowedCommandsUnknownNamespace(t *testing.T) {
	_, err := interp.New(interp.AllowedCommands([]string{"host:echo"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown namespace")
}

func TestAllowedCommandsEmptyCommandName(t *testing.T) {
	_, err := interp.New(interp.AllowedCommands([]string{"rshell:"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty command name")
}

func TestAllowedCommandsValidPrefix(t *testing.T) {
	_, err := interp.New(interp.AllowedCommands([]string{"rshell:echo"}))
	require.NoError(t, err)
}

func TestAllowedCommandsRejectsWhitespace(t *testing.T) {
	// Whitespace in entries is not trimmed (by design per specs).
	_, err := interp.New(
		interp.AllowedCommands([]string{" rshell:echo "}),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown namespace")
}

func TestAllowedCommandsMultipleColons(t *testing.T) {
	_, err := interp.New(interp.AllowedCommands([]string{"rshell:foo:bar"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple colons")
}

func TestAllowedCommandsDuplicateEntries(t *testing.T) {
	_, err := interp.New(interp.AllowedCommands([]string{"rshell:echo", "rshell:echo"}))
	require.NoError(t, err, "duplicate entries should be accepted (idempotent)")
}

func TestAllowedCommandsEmpty(t *testing.T) {
	_, err := interp.New(interp.AllowedCommands([]string{""}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty command name")
}
