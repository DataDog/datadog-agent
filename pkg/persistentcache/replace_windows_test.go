// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package persistentcache

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenamePathPtr(t *testing.T) {
	shortLocal := `C:\` + strings.Repeat("a", 244)
	prefixBoundary := `C:\` + strings.Repeat("a", 245)
	overMaxLocal := `C:\` + strings.Repeat("a", 130) + `\` + strings.Repeat("b", 130)
	overMaxUNC := `\\server\share\` + strings.Repeat("a", 120) + `\` + strings.Repeat("b", 130)
	require.Greater(t, len(overMaxLocal), 260)
	require.Greater(t, len(overMaxUNC), 260)

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "short local path", path: shortLocal, want: shortLocal},
		{name: "248-character prefix boundary", path: prefixBoundary, want: `\\?\` + prefixBoundary},
		{name: "local path beyond MAX_PATH", path: overMaxLocal, want: `\\?\` + overMaxLocal},
		{name: "UNC path beyond MAX_PATH", path: overMaxUNC, want: `\\?\UNC\` + strings.TrimPrefix(overMaxUNC, `\\`)},
		{name: "extended path", path: `\\?\C:\cache\file`, want: `\\?\C:\cache\file`},
		{name: "NT namespace path", path: `\??\C:\cache\file`, want: `\??\C:\cache\file`},
		{name: "device path", path: `\\.\COM1`, want: `\\.\COM1`},
		{name: "invalid path", path: "C:\\bad\x00path", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr, err := renamePathPtr(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, windows.UTF16PtrToString(ptr))
		})
	}
}
