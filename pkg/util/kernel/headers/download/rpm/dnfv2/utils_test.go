// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package dnfv2

import (
	_ "embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/repo"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/simple-1.0.1-1.i386.rpm
var testRPM []byte

func TestExtractPackageDoesNotStoreRPM(t *testing.T) {
	dir := t.TempDir()
	pkg := &repo.PkgInfo{Header: repo.PkgInfoHeader{Name: "headers"}}
	require.NoError(t, ExtractPackage(pkg, testRPM, dir, &types.Target{Uname: types.Utsname{Kernel: "test"}}, nil))
	regular := 0
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		require.NoError(t, err)
		require.False(t, strings.HasSuffix(path, ".rpm"))
		if entry.Type().IsRegular() {
			regular++
		}
		return nil
	}))
	require.Positive(t, regular)
	failed := t.TempDir()
	require.ErrorContains(t, ExtractPackage(pkg, []byte("invalid"), failed, &types.Target{Uname: types.Utsname{Kernel: "test"}}, nil), "headers-")
	entries, err := os.ReadDir(failed)
	require.NoError(t, err)
	require.Empty(t, entries)
}
