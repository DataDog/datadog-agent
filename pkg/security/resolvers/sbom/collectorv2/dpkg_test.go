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
