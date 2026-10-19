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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	status := filepath.Join(dir, "var/lib/dpkg/status")
	require.NoError(t, os.MkdirAll(filepath.Dir(status), 0o755))
	require.NoError(t, os.WriteFile(status, []byte("Package: bash\n"), 0o644))
	scanner := NewOSScanner()

	base := scanner.Fingerprint(dir)
	assert.NotEmpty(t, base)
	assert.Equal(t, base, scanner.Fingerprint(dir), "the fingerprint of an unchanged root changed")

	require.NoError(t, os.WriteFile(status, []byte("Package: bash\n\nPackage: curl\n"), 0o644))
	installed := scanner.Fingerprint(dir)
	assert.NotEqual(t, base, installed, "an install kept the fingerprint")

	require.NoError(t, os.Chtimes(status, time.Time{}, time.Now().Add(time.Hour)))
	assert.NotEqual(t, installed, scanner.Fingerprint(dir), "a rewrite of the same size kept the fingerprint")

	assert.Empty(t, scanner.Fingerprint(filepath.Join(dir, "missing")))
}

// writeRoot writes the files of a root, by their path in the root.
func writeRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

// TestScanInstalledPackagesLeavesOutDocs checks that the documentation of a
// package stays out of its index, so that reading it leaves its usage as it is.
func TestScanInstalledPackagesLeavesOutDocs(t *testing.T) {
	dir := writeRoot(t, map[string]string{
		"var/lib/dpkg/status": "Package: bash\nStatus: install ok installed\nVersion: 5.2.15-2\n",
		"var/lib/dpkg/info/bash.md5sums": strings.Join([]string{
			"0  bin/bash",
			"0  usr/share/doc/bash/copyright",
			"0  usr/share/info/bash.info.gz",
			"0  usr/share/licenses/bash/COPYING",
			"0  usr/share/locale/fr/LC_MESSAGES/bash.mo",
			"0  usr/share/man/man1/bash.1.gz",
			"0  usr/share/bash-completion/completions/bash",
		}, "\n") + "\n",
	})

	pkgs, err := NewOSScanner().ScanInstalledPackages(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, []string{"/bin/bash", "/usr/share/bash-completion/completions/bash"}, pkgs[0].InstalledFiles)
}

// TestScanInstalledPackagesLeavesOutDocsOfEachArchitecture checks that the dpkg
// entries of a multiarch package, which share their files, keep them once each.
func TestScanInstalledPackagesLeavesOutDocsOfEachArchitecture(t *testing.T) {
	dir := writeRoot(t, map[string]string{
		"var/lib/dpkg/status": "Package: libc6\nStatus: install ok installed\nArchitecture: amd64\nVersion: 2.36-9\n\n" +
			"Package: libc6\nStatus: install ok installed\nArchitecture: i386\nVersion: 2.36-9\n",
		"var/lib/dpkg/info/libc6:amd64.md5sums": "0  usr/lib/x86_64-linux-gnu/libc.so.6\n0  usr/share/doc/libc6/copyright\n",
		"var/lib/dpkg/info/libc6:i386.md5sums":  "0  usr/lib/i386-linux-gnu/libc.so.6\n0  usr/share/doc/libc6/changelog.gz\n",
	})

	pkgs, err := NewOSScanner().ScanInstalledPackages(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, pkgs, 2)
	for _, pkg := range pkgs {
		assert.ElementsMatch(t, []string{"/usr/lib/x86_64-linux-gnu/libc.so.6", "/usr/lib/i386-linux-gnu/libc.so.6"}, pkg.InstalledFiles)
	}
}
