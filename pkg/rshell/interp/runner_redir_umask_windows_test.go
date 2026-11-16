// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package interp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemediationRedirect_Umask on Windows just verifies redirect-created
// files exist and are writable. Windows has no Unix-style umask; Go maps the
// mode to the read-only attribute via perm&0200, which redirectFilePerm sets.
func TestRemediationRedirect_Umask(t *testing.T) {
	dir := t.TempDir()

	script := "echo trunc > trunc.txt\n" +
		"echo clob >| clob.txt\n" +
		"echo app >> app.txt\n" +
		"echo all &> all.txt\n" +
		"echo appall &>> appall.txt\n"

	r, _, stderr := newRemediationRunner(t, dir)
	require.NoError(t, r.Run(context.Background(), parseScript(t, script)),
		"stderr=%q", stderr.String())

	for _, name := range []string{"trunc.txt", "clob.txt", "app.txt", "all.txt", "appall.txt"} {
		info, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.NotZero(t, info.Mode().Perm()&0o200, "%s must not be read-only", name)
	}
}
