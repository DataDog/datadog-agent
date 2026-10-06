// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilesystemRootPathWindows(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "drive", path: `C:\Data\Scripts`, want: `C:\`},
		{name: "UNC share", path: `\\server\share\scripts`, want: `\\server\share\`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, filesystemRootPath(tt.path))
		})
	}
}
