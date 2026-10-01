// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePasswd(t *testing.T, dir, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "passwd"), []byte(contents), 0o600))
}

func TestLookupHostUserSkipsInvalidRowsAndUsesFirstMatch(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "\n# comment\nmalformed\n+compat:x:42:1::/:/bin/sh\n-invalid:x:42:2::/:/bin/sh\nbaduid:x:nope:3::/:/bin/sh\nincomplete:x:42:4\nfirst:x:00042:4::/:/bin/sh\nsecond:x:42:5::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)

	u := lookupHostUser("42")
	require.NotNil(t, u)
	assert.Equal(t, "first", u.Username)
	assert.Nil(t, lookupHostUser("43"))
}

func TestLookupHostUserStopsAfterMatch(t *testing.T) {
	dir := t.TempDir()
	// A matching record must not be discarded because an unreadable record
	// occurs later in the file.
	writePasswd(t, dir, "host-user:x:42:4::/:/bin/sh\n"+strings.Repeat("x", 1024*1024+1))
	t.Setenv("HOST_ETC", dir)

	u := lookupHostUser("42")
	require.NotNil(t, u)
	assert.Equal(t, "host-user", u.Username)
}
