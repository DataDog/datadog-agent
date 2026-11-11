// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package allowedpaths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxStatFSDoesNotRepeatExactHostPrefix(t *testing.T) {
	hostPrefix := t.TempDir()
	source := filepath.Join(hostPrefix, "source")
	require.NoError(t, os.Mkdir(source, 0o700))
	require.NoError(t, os.Symlink(hostPrefix, filepath.Join(source, "link")))

	sb, _, err := New([]string{hostPrefix, source})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetHostPrefix(hostPrefix)

	_, err = sb.StatFS("link", source)
	require.NoError(t, err)
}
