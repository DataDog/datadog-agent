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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDpkgListPackagesSkipsBadRecords(t *testing.T) {
	dir := t.TempDir()
	write := func(path, content string) {
		path = filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	write("var/lib/dpkg/status", "Package: a\nStatus: install ok installed\nVersion: 1.0-1\n\n"+
		"Package: big\nDescription: "+strings.Repeat("x", 200*1024)+"\n")
	write("var/lib/dpkg/info/a.md5sums", "\nnospace\n0123456789abcdef \n0123456789abcdef  usr/bin/a\n")
	write("var/lib/dpkg/status.d/c", "Package: c\nStatus: install ok installed\nVersion: 2.0\n")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "var/lib/dpkg/status.d/dir"), 0o755))
	require.NoError(t, os.Symlink("../../../../../outside", filepath.Join(dir, "var/lib/dpkg/status.d/escape")))

	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()

	pkgs, err := (&dpkgScanner{}).ListPackages(context.Background(), root)
	require.NoError(t, err)

	files := make(map[string][]string)
	for _, pkg := range pkgs {
		files[pkg.Name] = pkg.InstalledFiles
	}
	assert.Equal(t, map[string][]string{"a": {"/usr/bin/a"}, "c": nil}, files)
}

func TestDpkgInstalledFiles(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  map[string][]string
	}{
		{
			name: "md5sums alone",
			files: map[string]string{
				"var/lib/dpkg/status.d/a.md5sums": "0  usr/bin/a\n",
			},
			want: map[string][]string{"a": {"/usr/bin/a"}},
		},
		{
			name: "takeover",
			files: map[string]string{
				"var/lib/dpkg/info/old.md5sums": "0  usr/bin/shared\n0  usr/share/old/own\n",
				"var/lib/dpkg/info/old.list":    "/.\n/usr\n/usr/share\n/usr/share/old\n/usr/share/old/own\n",
				"var/lib/dpkg/info/new.md5sums": "0  usr/bin/shared\n",
				"var/lib/dpkg/info/new.list":    "/.\n/usr\n/usr/bin\n/usr/bin/shared\n",
			},
			want: map[string][]string{
				"old": {"/usr/share/old/own"},
				"new": {"/usr/bin/shared"},
			},
		},
		{
			name: "multiarch takeover",
			files: map[string]string{
				"var/lib/dpkg/info/lib:amd64.md5sums": "0  usr/lib/x86_64-linux-gnu/lib.so\n0  usr/share/lib/moved\n",
				"var/lib/dpkg/info/lib:amd64.list":    "/usr/lib/x86_64-linux-gnu/lib.so\n",
			},
			want: map[string][]string{"lib": {"/usr/lib/x86_64-linux-gnu/lib.so"}},
		},
		{
			name: "diversions",
			files: map[string]string{
				"var/lib/dpkg/info/man-db.md5sums":   "0  usr/bin/man\n0  usr/bin/tool\n",
				"var/lib/dpkg/info/diverter.md5sums": "0  usr/bin/tool\n",
				"var/lib/dpkg/diversions":            "/usr/bin/man\n/usr/bin/man.REAL\n:\n/usr/bin/tool\n/usr/bin/tool.distrib\ndiverter\n",
			},
			want: map[string][]string{
				"man-db":   {"/usr/bin/man.REAL", "/usr/bin/tool.distrib"},
				"diverter": {"/usr/bin/tool"},
			},
		},
		{
			name: "dot prefix",
			files: map[string]string{
				"var/lib/dpkg/info/gh.md5sums": "0  ./usr/bin/gh\n",
				"var/lib/dpkg/info/gh.list":    "/.\n/usr\n/usr/bin\n/usr/bin/gh\n",
			},
			want: map[string][]string{"gh": {"/usr/bin/gh"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				path := filepath.Join(dir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
			}
			root, err := os.OpenRoot(dir)
			require.NoError(t, err)
			defer root.Close()

			got, err := (&dpkgScanner{}).listInstalledFiles(root)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
