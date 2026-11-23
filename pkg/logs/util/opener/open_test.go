// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package opener

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestForSourceUsesCurrentPolicy(t *testing.T) {
	const path = "app.log"
	makeSource := func(noFollow bool) *sources.LogSource {
		return sources.NewLogSource("", &config.LogsConfig{NoFollow: noFollow})
	}

	base := NewMockFileOpener()
	base.AddMockFile(NewMockFile(path, [][]byte{[]byte("line\n")}))
	source := sources.NewReplaceableSource(makeSource(false))
	fileOpener := ForSource(base, source)

	for _, noFollow := range []bool{false, true, false} {
		source.Replace(makeSource(noFollow))
		f, err := fileOpener.OpenLogFile(path)
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}

	require.Equal(t, []LogFileOpen{
		{Path: path},
		{Path: path, NoFollow: true},
		{Path: path},
	}, base.Opens())
}

func TestNoFollowIsIdempotent(t *testing.T) {
	const path = "app.log"
	base := NewMockFileOpener()
	base.AddMockFile(NewMockFile(path, [][]byte{[]byte("line\n")}))

	f, err := base.NoFollow().NoFollow().OpenLogFile(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.Equal(t, []LogFileOpen{{Path: path, NoFollow: true}}, base.Opens())
}

func TestMockFileReadAt(t *testing.T) {
	file := NewMockFile("app.log", [][]byte{[]byte("ab"), []byte("cd")})

	buf := make([]byte, 3)
	n, err := file.ReadAt(buf, 1)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, []byte("bcd"), buf)
	require.Equal(t, 0, file.CurrentPos())

	buf = make([]byte, 3)
	n, err = file.ReadAt(buf, 3)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 1, n)
	require.Equal(t, byte('d'), buf[0])
	require.Equal(t, 0, file.CurrentPos())
}
