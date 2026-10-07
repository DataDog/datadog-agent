// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package collectorv2

import (
	"testing"

	rpmdb "github.com/knqyf263/go-rpmdb/pkg"
	"github.com/stretchr/testify/assert"
)

func TestIndexedRPMFiles(t *testing.T) {
	installed := []rpmdb.FileInfo{
		{Path: "/etc/passwd", Flags: rpmdb.FileFlags(rpmdb.RPMFILE_CONFIG | rpmdb.RPMFILE_NOREPLACE)},
		{Path: "/etc/ld.so.cache", Flags: rpmdb.FileFlags(rpmdb.RPMFILE_GHOST)},
		{Path: "/usr/share/doc/glibc/README", Flags: rpmdb.FileFlags(rpmdb.RPMFILE_DOC)},
		{Path: "/usr/share/licenses/glibc/COPYING", Flags: rpmdb.FileFlags(rpmdb.RPMFILE_LICENSE)},
		{Path: "/usr/lib64/libc.so.6"},
		{Path: "/usr/lib64", Mode: 040755},
	}

	assert.Equal(t, []string{"/usr/lib64/libc.so.6", "/usr/lib64"}, indexedRPMFiles(installed))
}
