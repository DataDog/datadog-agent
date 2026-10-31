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

	"github.com/stretchr/testify/assert"
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

func TestCorrectPlatform(t *testing.T) {
	cases := []struct {
		v        string
		in       platformInfo
		expected platformInfo
	}{
		{"6.8.0-1063-aws", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"ubuntu", "debian", "24.04"}},
		{"6.12.73-95.123.amzn2023.x86_64", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"amazon", "rhel", "2023"}},
		{"5.10.245-245.983.amzn2.x86_64", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"amazon", "rhel", "2"}},
		// ensure it doesn't adjust correct values
		{"5.10.245-245.983.amzn2.x86_64", platformInfo{"amazon", "rhel", "2"}, platformInfo{"amazon", "rhel", "2"}},
		{"6.7.4-200.fc39.x86_64", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"fedora", "fedora", "39"}},
		{"5.15.0-317.197.5.1.el8uek.x86_64", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"oracle", "rhel", "8"}},
		{"5.15.0-317.197.5.1.el8uek.x86_64", platformInfo{"ol", "", "8.10"}, platformInfo{"oracle", "rhel", "8.10"}},
		{"5.14.0-570.128.1.el9_6.x86_64", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"redhat", "rhel", "9.6"}},
		{"6.12.0-254.el10.x86_64", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"redhat", "rhel", "10"}},
		// ensure it doesn't correct a potentially correct platform
		{"6.12.0-254.el10.x86_64", platformInfo{"centos", "rhel", "10"}, platformInfo{"centos", "rhel", "10"}},
		{"5.10.0-0.deb10.17-arm64.btf.tar.xz", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"debian", "debian", "10"}},
		{"6.12.43+deb13-amd64", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"debian", "debian", "13"}},
		{"4.12.14-lp151.27-default.btf.tar.xz", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"opensuse-leap", "suse", "15.1"}},
		{"5.3.18-150300.59.81-64kb.btf.tar.xz", platformInfo{"ubuntu", "debian", "24.04"}, platformInfo{"sles", "suse", "15.3"}},
	}
	for _, c := range cases {
		out := correctPlatform(c.in, c.v)
		assert.Equal(t, c.expected, out, "%s correction", c.v)
	}
}
