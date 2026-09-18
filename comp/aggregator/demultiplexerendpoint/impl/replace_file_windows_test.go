// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package demultiplexerendpointimpl

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

func TestReplaceFileWhileDestinationIsOpen(t *testing.T) {
	runPath := t.TempDir()
	destinationPath := filepath.Join(runPath, "dogstatsd_contexts.json.zstd")
	sourcePath := filepath.Join(runPath, ".dogstatsd_contexts.tmp")
	require.NoError(t, os.WriteFile(destinationPath, []byte("previous dump"), 0644))
	require.NoError(t, os.WriteFile(sourcePath, []byte("new dump"), 0644))

	reader, err := filesystem.OpenShared(destinationPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	require.NoError(t, replaceFile(sourcePath, destinationPath))

	contents, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, []byte("previous dump"), contents)

	contents, err = os.ReadFile(destinationPath)
	require.NoError(t, err)
	require.Equal(t, []byte("new dump"), contents)

	_, err = os.Stat(sourcePath)
	require.ErrorIs(t, err, os.ErrNotExist)
}
