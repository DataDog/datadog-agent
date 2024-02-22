// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlatform(t *testing.T) {
	osr, err := os.Open("/etc/os-release")
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		t.Skip("/etc/os-release does not exist")
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = osr.Close() })

	tmp := t.TempDir()
	t.Setenv("HOST_ETC", tmp)
	require.NoError(t, os.Mkdir(filepath.Join(tmp, "redhat-release"), 0755))

	// copy /etc/os-release to <tmpdir>/os-release
	dosr, err := os.Create(filepath.Join(tmp, "os-release"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = dosr.Close() })
	_, err = io.Copy(dosr, osr)
	require.NoError(t, err)
	_ = dosr.Close()

	pi, err := getPlatformInformation()
	require.NoError(t, err)
	require.NotEmpty(t, pi.platform, "platform")
	require.NotEmpty(t, pi.family, "family")
	require.NotEmpty(t, pi.version, "version")
}
