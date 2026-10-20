// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package ntfsdu_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// On non-Windows platforms ntfs-du must not be registered, so it never appears
// in the builtin registry (and therefore not in `help`). Constructing a runner
// triggers builtin registration.
func TestNotRegisteredOffWindows(t *testing.T) {
	r, err := interp.New()
	require.NoError(t, err)
	r.Close()

	require.False(t, slices.Contains(builtins.Names(), "ntfs-du"),
		"ntfs-du must not be registered on non-Windows platforms")
}
