// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procmaps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// writeFakeProc builds a fake /proc/<pid>/{comm,maps,smaps} tree under a
// temp directory and returns the fabricated proc root path.
func writeFakeProc(t *testing.T, pid int, comm, maps, smaps string) string {
	t.Helper()
	procPath := t.TempDir()
	pidDir := filepath.Join(procPath, strconv.Itoa(pid))
	assert.NoError(t, os.MkdirAll(pidDir, 0o755))
	if comm != "" {
		assert.NoError(t, os.WriteFile(filepath.Join(pidDir, "comm"), []byte(comm), 0o644))
	}
	if maps != "" {
		assert.NoError(t, os.WriteFile(filepath.Join(pidDir, "maps"), []byte(maps), 0o644))
	}
	if smaps != "" {
		assert.NoError(t, os.WriteFile(filepath.Join(pidDir, "smaps"), []byte(smaps), 0o644))
	}
	return procPath
}

const sampleMaps = `00400000-00452000 r-xp 00000000 08:01 173521 /usr/bin/example
00651000-00652000 r--p 00051000 08:01 173521 /usr/bin/example
00e03000-00e24000 rw-p 00000000 00:00 0 [heap]
7f4b0c000000-7f4b0c021000 rw-p 00000000 00:00 0
7ffe0dd6b000-7ffe0dd8c000 rw-p 00000000 00:00 0 [stack]
`

func TestReadImpl_HappyPath(t *testing.T) {
	procPath := writeFakeProc(t, 42, "example\n", sampleMaps, "")
	name, mappings, err := readImpl(context.Background(), procPath, 42, false)
	assert.NoError(t, err)
	assert.Equal(t, "example", name)
	assert.Len(t, mappings, 5)

	assert.Equal(t, uint64(0x00400000), mappings[0].Start)
	assert.Equal(t, uint64(0x00452000), mappings[0].End)
	assert.Equal(t, "r-x--", mappings[0].Perms)
	assert.Equal(t, "example", mappings[0].Name)

	assert.Equal(t, "[heap]", mappings[2].Name)
	assert.Equal(t, "[ anon ]", mappings[3].Name)
	assert.Equal(t, "[stack]", mappings[4].Name)
}

func TestReadImpl_NoSuchProcess(t *testing.T) {
	procPath := t.TempDir()
	_, _, err := readImpl(context.Background(), procPath, 99999, false)
	assert.True(t, errors.Is(err, ErrNoSuchProcess))
}

func TestReadImpl_ContextCancelled(t *testing.T) {
	procPath := writeFakeProc(t, 7, "example\n", sampleMaps, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, mappings, err := readImpl(ctx, procPath, 7, false)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, mappings)
}

const sampleSmaps = `00400000-00452000 r-xp 00000000 08:01 173521 /usr/bin/example
Rss:                 200 kB
Private_Dirty:        50 kB
Shared_Dirty:          10 kB
00e03000-00e24000 rw-p 00000000 00:00 0 [heap]
Rss:                 132 kB
Private_Dirty:       132 kB
Shared_Dirty:          0 kB
`

func TestReadImpl_ExtendedHappyPath(t *testing.T) {
	procPath := writeFakeProc(t, 42, "example\n", "", sampleSmaps)
	name, mappings, err := readImpl(context.Background(), procPath, 42, true)
	assert.NoError(t, err)
	assert.Equal(t, "example", name)
	assert.Len(t, mappings, 2)

	assert.True(t, mappings[0].HasRSS)
	assert.Equal(t, uint64(200), mappings[0].RSSKB)
	assert.Equal(t, uint64(60), mappings[0].DirtyKB)

	assert.Equal(t, uint64(132), mappings[1].RSSKB)
	assert.Equal(t, uint64(132), mappings[1].DirtyKB)
}

func TestReadImpl_ExtendedRejectsTruncatedRecord(t *testing.T) {
	truncated := `00400000-00452000 r-xp 00000000 08:01 173521 /usr/bin/example
Rss:                 200 kB
Private_Dirty:        50 kB
`
	procPath := writeFakeProc(t, 42, "example\n", "", truncated)
	_, mappings, err := readImpl(context.Background(), procPath, 42, true)
	assert.ErrorIs(t, err, ErrMalformedData)
	assert.Empty(t, mappings)
}

func TestMaxMappingsCap(t *testing.T) {
	var b []byte
	for i := 0; i < MaxMappings+10; i++ {
		start := i * 0x1000
		end := start + 0x1000
		b = append(b, []byte(hexRange(start, end)+" rw-p 00000000 00:00 0\n")...)
	}
	procPath := writeFakeProc(t, 1, "example\n", string(b), "")
	_, mappings, err := readImpl(context.Background(), procPath, 1, false)
	assert.ErrorIs(t, err, ErrMappingLimitExceeded)
	assert.Empty(t, mappings)
}

func hexRange(start, end int) string {
	return strconv.FormatInt(int64(start), 16) + "-" + strconv.FormatInt(int64(end), 16)
}

func TestMapsPermsToMode(t *testing.T) {
	cases := map[string]string{
		"r-xp": "r-x--",
		"rw-p": "rw---",
		"r--s": "r--s-",
		"----": "-----",
	}
	for in, want := range cases {
		assert.Equal(t, want, mapsPermsToMode(in), "input %q", in)
	}
}

func TestParseAddrRange(t *testing.T) {
	start, end, ok := parseAddrRange("00400000-00452000")
	assert.True(t, ok)
	assert.Equal(t, uint64(0x00400000), start)
	assert.Equal(t, uint64(0x00452000), end)

	_, _, ok = parseAddrRange("not-a-range")
	assert.False(t, ok)

	_, _, ok = parseAddrRange("nodash")
	assert.False(t, ok)
}

func TestParseMapsLineRejectsReversedRange(t *testing.T) {
	_, ok := parseMapsLine("00452000-00400000 r-xp 00000000 08:01 173521 /usr/bin/example")
	assert.False(t, ok)
}

func TestReadCommIsBounded(t *testing.T) {
	procPath := writeFakeProc(t, 42, strings.Repeat("x", maxCommBytes+1), sampleMaps, "")
	_, _, err := readImpl(context.Background(), procPath, 42, false)
	assert.ErrorIs(t, err, ErrMalformedData)
}

func TestReadCommSanitizesControlCharacters(t *testing.T) {
	procPath := writeFakeProc(t, 42, "line\nbreak\tname\n", sampleMaps, "")
	name, _, err := readImpl(context.Background(), procPath, 42, false)
	assert.NoError(t, err)
	assert.Equal(t, "line?break?name", name)
}
