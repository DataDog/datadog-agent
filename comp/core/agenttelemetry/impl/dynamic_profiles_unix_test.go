// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package agenttelemetryimpl

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cache is shared by agent processes running as different users (core agent as
// dd-agent, system-probe and security-agent as root), so it must stay world readable.
// os.CreateTemp makes 0600, hence the explicit chmod; and unlike os.WriteFile's perm
// argument, chmod is not subject to the process umask.
func TestDynamicProfilesCache_WrittenWorldReadable(t *testing.T) {
	old := syscall.Umask(0077)
	t.Cleanup(func() { syscall.Umask(old) })

	path := t.TempDir() + "/doc.json"
	require.NoError(t, writeCachedDocument(path, &cachedDocument{Config: testDynamicProfilesDoc}))

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), fi.Mode().Perm())
}
