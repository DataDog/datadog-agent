// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package detectors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/languagedetection"
)

func TestDotnetMapsParser(t *testing.T) {
	data := []struct {
		name   string
		maps   string
		result bool
	}{
		{
			name: "empty maps",
			maps: "",
		},
		{
			name: "not in maps",
			maps: `
79f6cd47d000-79f6cd47f000 r--p 00000000 fc:04 793163                     /usr/lib/python3.10/lib-dynload/_bz2.cpython-310-x86_64-linux-gnu.so
79f6cd479000-79f6cd47a000 r-xp 00001000 fc:06 5507018                    /home/foo/.local/lib/python3.10/site-packages/ddtrace_fake/md.cpython-310-x86_64-linux-gnu.so
			`,
			result: false,
		},
		{
			name: "in maps",
			maps: `
7d97b4e57000-7d97b4e85000 r--s 00000000 fc:04 1332568                    /usr/lib/dotnet/shared/Microsoft.NETCore.App/8.0.8/System.Con
sole.dll
7d97b4e85000-7d97b4e8e000 r--s 00000000 fc:04 1332665                    /usr/lib/dotnet/shared/Microsoft.NETCore.App/8.0.8/System.Runtime.dll
7d97b4e8e000-7d97b4e99000 r--p 00000000 fc:04 1332718                    /usr/lib/dotnet/shared/Microsoft.NETCore.App/8.0.8/libSystem.Native.so
			`,
			result: true,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result, err := mapsHasDotnetDll(strings.NewReader(d.maps))
			assert.NoError(t, err)
			assert.Equal(t, d.result, result)
		})
	}
}

func TestDotnetDetector(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	dll := filepath.Join(curDir, "testdata", "System.Runtime.dll")
	cmd, err := fileopener.OpenFromAnotherProcess(t, dll)
	require.NoError(t, err)

	proc := &languagedetection.Process{Pid: int32(cmd.Process.Pid)}
	langInfo, err := NewDotnetDetector().DetectLanguage(proc)
	require.NoError(t, err)
	assert.Equal(t, languagemodels.Dotnet, langInfo.Name)

	self := &languagedetection.Process{Pid: int32(os.Getpid())}
	_, err = NewDotnetDetector().DetectLanguage(self)
	require.Error(t, err)
}
