// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

package headers

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

func TestDownloadKernelHeaders(t *testing.T) {
	flake.Mark(t) // flaky because it reaches out to real package repos
	ebpftest.LogLevel(t, "debug")
	t.Cleanup(func() { HeaderProvider = nil })

	opts := HeaderOptions{
		DownloadEnabled: true,
		Dirs:            []string{t.TempDir()},
		DownloadDir:     t.TempDir(),

		AptConfigDir:   "/etc/apt",
		YumReposDir:    "/etc/yum.repos.d",
		ZypperReposDir: "/etc/zypp/repos.d",
	}
	dirs := GetKernelHeaders(opts)
	assert.NotZero(t, len(dirs), "expected to find header directories")
	t.Log(dirs)

	result := HeaderProvider.GetResult()
	assert.Equal(t, result.IsSuccess(), true)
}

func TestParseHeaderVersion(t *testing.T) {
	cases := []struct {
		body string
		v    kernel.Version
		err  bool
	}{
		{"#define LINUX_VERSION_CODE 328769", kernel.Version(328769), false},
		{"#define  LINUX_VERSION_CODE		123456", kernel.Version(123456), false},
		{"#define LINUX_VERSION_CODE -1", kernel.Version(0), true},
		{"#define LINUX_VERSION_CODE", kernel.Version(0), true},
		{"", kernel.Version(0), true},
	}

	for _, c := range cases {
		hv, err := parseHeaderVersion(bytes.NewBufferString(c.body))
		if c.err {
			assert.Error(t, err, "expected error parsing of `%s`", c.body)
		} else {
			if assert.NoError(t, err, "parse error of `%s`", c.body) {
				assert.Equal(t, c.v, hv, "version mismatch of `%s`", c.body)
			}
		}
	}
}
