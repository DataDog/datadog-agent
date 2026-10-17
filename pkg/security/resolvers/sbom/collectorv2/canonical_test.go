// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package collectorv2

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mergedRoot builds a root with a merged /usr and a /usr/sbin merged into
// /usr/bin, as on Fedora 42 and RHEL 10.
func mergedRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"usr/bin", "usr/lib/x86_64-linux-gnu", "etc", "opt/releases/v1", "opt/releases/shared"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, d), 0o755))
	}
	for link, target := range map[string]string{
		"bin":         "usr/bin",
		"lib":         "usr/lib",
		"usr/sbin":    "bin",
		"abs":         "/usr/lib",
		"loop":        "loop",
		"usr/bin/sh":  "bash",
		"dangling":    "nowhere",
		"usr/lib/up":  "../bin",
		"usr/lib/out": "../../../../usr/lib",
		"opt/current": "releases/v1",
		"alias":       "/opt/current/../shared",
	} {
		require.NoError(t, os.Symlink(target, filepath.Join(dir, link)))
	}
	return dir
}

func TestDirResolver(t *testing.T) {
	root, err := os.OpenRoot(mergedRoot(t))
	require.NoError(t, err)
	defer root.Close()

	d := newDirResolver(root)
	for listed, want := range map[string]string{
		"/bin/bash":                          "/usr/bin/bash",
		"/usr/sbin/ip":                       "/usr/bin/ip",
		"/lib/x86_64-linux-gnu/libc.so.6":    "/usr/lib/x86_64-linux-gnu/libc.so.6",
		"/abs/x86_64-linux-gnu/libc.so.6":    "/usr/lib/x86_64-linux-gnu/libc.so.6",
		"/usr/lib/up/ls":                     "/usr/bin/ls",
		"/usr/lib/out/x86_64-linux-gnu/libz": "/usr/lib/x86_64-linux-gnu/libz",
		"/alias/tool":                        "/opt/releases/shared/tool",
		"/usr/bin/sh":                        "/usr/bin/sh",
		"/etc/hosts":                         "/etc/hosts",
		"/loop/x":                            "/loop/x",
		"/dangling/x":                        "/nowhere/x",
		"/missing/dir/x":                     "/missing/dir/x",
	} {
		assert.Equal(t, want, d.path(listed), listed)
	}
}

func TestScanInstalledPackagesResolvesDirectories(t *testing.T) {
	dir := mergedRoot(t)
	status := filepath.Join(dir, "var/lib/dpkg/status")
	require.NoError(t, os.MkdirAll(filepath.Dir(status), 0o755))
	require.NoError(t, os.WriteFile(status, []byte("Package: bash\nStatus: install ok installed\nVersion: 5.2.15-2\n"), 0o644))
	md5sums := filepath.Join(dir, "var/lib/dpkg/info/bash.md5sums")
	require.NoError(t, os.MkdirAll(filepath.Dir(md5sums), 0o755))
	require.NoError(t, os.WriteFile(md5sums, []byte("0123456789abcdef  bin/bash\n"), 0o644))

	pkgs, err := NewOSScanner().ScanInstalledPackages(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, []string{"/usr/bin/bash"}, pkgs[0].InstalledFiles)
}
