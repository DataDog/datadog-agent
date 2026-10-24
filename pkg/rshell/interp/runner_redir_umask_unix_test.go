// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package interp

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemediationRedirect_Umask verifies that files created by write
// redirections get 0666 & ~umask, matching bash, rather than a hardcoded mode.
//
// Both umask values matter: at 022 the buggy hardcoded 0644 coincides with the
// correct result, so only the 002 case actually catches the divergence.
//
// syscall.Umask is process-global, so this test must not run in parallel and
// must restore the previous value.
func TestRemediationRedirect_Umask(t *testing.T) {
	cases := []struct {
		name  string
		umask int
		want  os.FileMode
	}{
		{name: "umask022", umask: 0o022, want: 0o644},
		{name: "umask002", umask: 0o002, want: 0o664},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Deliberately not t.Parallel(): umask is process-global.
			dir := t.TempDir()

			old := syscall.Umask(tc.umask)
			defer syscall.Umask(old)

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
				perm := info.Mode().Perm()
				assert.Equal(t, tc.want, perm,
					"%s: expected %04o (0666 & ~%04o), got %04o", name, tc.want, tc.umask, perm)
			}
		})
	}
}
