// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package darwin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeduperCollapsesShellShimReexec is the case this exists for. macOS /bin/sh
// is bash in sh-compat mode and immediately re-execs itself as /bin/bash, so one
// logical shell spawn produces two exec events at the same pid. Both genuinely
// match a shell rule, so the rule engine fires twice and a single action shows up
// as two signals.
func TestDeduperCollapsesShellShimReexec(t *testing.T) {
	tr := newTestTranslator(t)

	d, err := newSignalDeduper()
	require.NoError(t, err)

	// The real sequence: /bin/sh execs at pid 4100, then /bin/bash at the same pid.
	shEvent, err := tr.Translate(execMessage(t, 4100, 4099, "/bin/sh", []string{"sh", "-c", "echo hi"}))
	require.NoError(t, err)
	require.NotNil(t, shEvent)
	assert.True(t, d.allow("macos_pkg_manager_spawns_shell", shEvent),
		"the first shell exec must be reported")

	bashEvent, err := tr.Translate(execMessage(t, 4100, 4099, "/bin/bash", []string{"sh", "-c", "echo hi"}))
	require.NoError(t, err)
	require.NotNil(t, bashEvent)
	assert.False(t, d.allow("macos_pkg_manager_spawns_shell", bashEvent),
		"the shim re-exec at the same pid must be collapsed into the first signal")
}

// TestDeduperKeepsDistinctPids checks the dedupe is not so aggressive that it
// hides separate activity: two different processes matching the same rule are two
// findings.
func TestDeduperKeepsDistinctPids(t *testing.T) {
	tr := newTestTranslator(t)

	d, err := newSignalDeduper()
	require.NoError(t, err)

	first, err := tr.Translate(execMessage(t, 4200, 1, "/bin/sh", []string{"sh"}))
	require.NoError(t, err)
	second, err := tr.Translate(execMessage(t, 4201, 1, "/bin/sh", []string{"sh"}))
	require.NoError(t, err)

	assert.True(t, d.allow("macos_pkg_manager_spawns_shell", first))
	assert.True(t, d.allow("macos_pkg_manager_spawns_shell", second),
		"a different pid is a different finding and must not be collapsed")
}

// TestDeduperIsPerRule checks that two different rules matching the same process
// are both reported, since they say different things about it.
func TestDeduperIsPerRule(t *testing.T) {
	tr := newTestTranslator(t)

	d, err := newSignalDeduper()
	require.NoError(t, err)

	ev, err := tr.Translate(execMessage(t, 4300, 1, "/bin/sh", []string{"sh"}))
	require.NoError(t, err)

	assert.True(t, d.allow("macos_pkg_manager_spawns_shell", ev))
	assert.True(t, d.allow("macos_pkg_manager_reads_credentials", ev),
		"a different rule about the same process is a different finding")
	assert.False(t, d.allow("macos_pkg_manager_spawns_shell", ev),
		"the same rule for the same pid is still a duplicate")
}
