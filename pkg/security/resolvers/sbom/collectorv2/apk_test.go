// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package collectorv2

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleAPKDatabase = `C:Q1r0+sample==
P:musl
V:1.2.3-r4
A:x86_64
S:1234
I:5678
T:the musl c library
U:https://musl.libc.org/
L:MIT
o:musl
m:Maintainer
t:1234567890
c:abcdef
D:so:libc.musl-x86_64.so.1
F:lib
R:ld-musl-x86_64.so.1
a:0:0:0755
Z:Q1abc
R:libc.musl-x86_64.so.1
a:0:0:0777
Z:Q1def
F:usr/lib
R:libc.musl-x86_64.so.1
a:0:0:0777
Z:Q1ghi

C:Q2r0+sample==
P:zlib
V:1.3.0-r0
A:x86_64
S:100
I:200
T:compression library
U:https://zlib.net/
L:zlib
o:zlib
F:lib
R:libz.so.1
a:0:0:0755
Z:Q1zzz
R:libz.so.1.3.0
a:0:0:0755
Z:Q1yyy

`

func TestParseAPKDatabase(t *testing.T) {
	pkgs, err := parseAPKDatabase(strings.NewReader(sampleAPKDatabase))
	require.NoError(t, err)
	require.Len(t, pkgs, 2)

	musl := pkgs[0]
	assert.Equal(t, "musl", musl.Name)
	assert.Equal(t, "1.2.3", musl.Version)
	assert.Equal(t, 0, musl.Epoch)
	assert.Equal(t, "r4", musl.Release)
	assert.Equal(t, "1.2.3", musl.SrcVersion)
	assert.Equal(t, 0, musl.SrcEpoch)
	assert.Equal(t, "r4", musl.SrcRelease)
	assert.Equal(t, []string{
		"/lib/ld-musl-x86_64.so.1",
		"/lib/libc.musl-x86_64.so.1",
		"/usr/lib/libc.musl-x86_64.so.1",
	}, musl.InstalledFiles)

	zlib := pkgs[1]
	assert.Equal(t, "zlib", zlib.Name)
	assert.Equal(t, "1.3.0", zlib.Version)
	assert.Equal(t, 0, zlib.Epoch)
	assert.Equal(t, "r0", zlib.Release)
	assert.Equal(t, []string{
		"/lib/libz.so.1",
		"/lib/libz.so.1.3.0",
	}, zlib.InstalledFiles)
}

func TestParseAPKDatabaseNoTrailingNewline(t *testing.T) {
	// File that doesn't end with an empty line — last package must still be captured
	db := strings.TrimRight(sampleAPKDatabase, "\n")
	pkgs, err := parseAPKDatabase(strings.NewReader(db))
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
}

func TestParseAPKDatabaseEmpty(t *testing.T) {
	pkgs, err := parseAPKDatabase(strings.NewReader(""))
	require.NoError(t, err)
	assert.Empty(t, pkgs)
}

func TestParseAPKVersion(t *testing.T) {
	tests := []struct {
		input          string
		expectedEpoch  int
		expectedVersion string
		expectedRelease string
	}{
		{"1.2.3-r4", 0, "1.2.3", "r4"},
		{"1.3.0-r0", 0, "1.3.0", "r0"},
		{"2:1.2.3-r4", 2, "1.2.3", "r4"},
		{"1.2.3", 0, "1.2.3", ""},
		{"1.2.3_alpha1-r0", 0, "1.2.3_alpha1", "r0"},
		{"20230101-r0", 0, "20230101", "r0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			epoch, version, release := parseAPKVersion(tt.input)
			assert.Equal(t, tt.expectedEpoch, epoch)
			assert.Equal(t, tt.expectedVersion, version)
			assert.Equal(t, tt.expectedRelease, release)
		})
	}
}
