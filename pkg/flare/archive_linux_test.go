// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package flare

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestZipLinuxFileWrapper(t *testing.T) {
	srcDir, err := ioutil.TempDir("", "ZipLinuxFileSource")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	dstDir, err := ioutil.TempDir("", "ZipLinuxFileTarget")
	require.NoError(t, err)
	defer os.RemoveAll(dstDir)

	file, err := os.Create(filepath.Join(srcDir, "testfile.txt"))
	require.NoError(t, err)

	expectedContent := "expected content"
	_, err = file.WriteString(expectedContent)
	require.NoError(t, err)

	err = file.Close()
	require.NoError(t, err)

	err = zipLinuxFile(srcDir, dstDir, "hostname", "testfile.txt")
	require.NoError(t, err)

	targetPath := filepath.Join(dstDir, "hostname", "testfile.txt")
	actualContent, err := ioutil.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, expectedContent, string(actualContent))
}
