// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ss_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/ss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var procPathMu sync.Mutex

func socketProcFixture(t *testing.T) string {
	t.Helper()
	procPathMu.Lock()
	t.Cleanup(procPathMu.Unlock)
	root := t.TempDir()
	netDir := filepath.Join(root, "net")
	require.NoError(t, os.Mkdir(netDir, 0700))
	files := map[string]string{
		"tcp":  "header\n0: 0100007F:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000 1000 0 123\n",
		"udp":  "header\n0: 0100007F:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000 1000 0 124\n",
		"tcp6": "header\n",
		"udp6": "header\n",
		"unix": "header\n00000000: 00000002 00000000 00010000 0001 0A 125 /test.sock\n",
	}
	for name, data := range files {
		require.NoError(t, os.WriteFile(filepath.Join(netDir, name), []byte(data), 0600))
	}
	old := ss.ProcPath
	ss.ProcPath = root
	t.Cleanup(func() { ss.ProcPath = old })
	return netDir
}

func TestSSLinuxMissingIPv6Tables(t *testing.T) {
	for _, missing := range [][]string{{"tcp6"}, {"udp6"}, {"tcp6", "udp6"}} {
		t.Run(strings.Join(missing, "+"), func(t *testing.T) {
			netDir := socketProcFixture(t)
			commands := []string{"ss -an", "ss -tanH", "ss -uan", "ss -s", "ss -an4", "ss -tan6H", "ss -uan6H"}
			expected := make(map[string]string)
			for _, command := range commands {
				stdout, stderr, code := cmdRun(t, command)
				require.Equal(t, 0, code, stderr)
				expected[command] = stdout
			}
			require.Contains(t, expected["ss -an"], "127.0.0.1:80")
			require.Contains(t, expected["ss -an"], "127.0.0.1:53")
			require.Contains(t, expected["ss -an"], "/test.sock")
			require.Contains(t, expected["ss -s"], "Total: 3\n")
			require.Empty(t, expected["ss -tan6H"])
			require.Empty(t, expected["ss -uan6H"])
			for _, name := range missing {
				require.NoError(t, os.Remove(filepath.Join(netDir, name)))
			}
			for _, command := range commands {
				t.Run(command, func(t *testing.T) {
					stdout, stderr, code := cmdRun(t, command)
					assert.Equal(t, 0, code)
					assert.Empty(t, stderr)
					assert.Equal(t, expected[command], stdout)
				})
			}
		})
	}
}

func TestSSLinuxProcTableErrors(t *testing.T) {
	for _, name := range []string{"tcp", "udp", "unix", "tcp6", "udp6"} {
		failures := []string{"missing"}
		if strings.HasSuffix(name, "6") {
			failures = []string{"permission", "directory", "oversized-line"}
		}
		for _, failure := range failures {
			t.Run(name+"/"+failure, func(t *testing.T) {
				netDir := socketProcFixture(t)
				path := filepath.Join(netDir, name)
				switch failure {
				case "missing":
					require.NoError(t, os.Remove(path))
				case "permission":
					if os.Geteuid() == 0 {
						t.Skip("root bypasses file permissions")
					}
					require.NoError(t, os.Chmod(path, 0000))
				case "directory":
					require.NoError(t, os.Remove(path))
					require.NoError(t, os.Mkdir(path, 0700))
				case "oversized-line":
					require.NoError(t, os.WriteFile(path, []byte("header\n"+strings.Repeat("x", ss.MaxLineBytes+1)), 0600))
				}
				stdout, stderr, code := cmdRun(t, "ss -an")
				assert.Equal(t, 1, code)
				assert.Empty(t, stdout)
				assert.Contains(t, stderr, "ss:")
			})
		}
	}
}
