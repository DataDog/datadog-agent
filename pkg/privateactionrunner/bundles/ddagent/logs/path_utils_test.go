// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeAndResolvePath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{
			name:  "simple absolute path",
			input: "/var/log/app.log",
			want:  "/host/var/log/app.log",
		},
		{
			name:  "path with redundant slashes",
			input: "/var//log///app.log",
			want:  "/host/var/log/app.log",
		},
		{
			name:  "path with dot segments",
			input: "/var/log/../log/app.log",
			want:  "/host/var/log/app.log",
		},
		{
			name:      "relative path",
			input:     "var/log/app.log",
			wantError: true,
		},
		{
			name:  "traversal collapsed to valid path",
			input: "/../../etc/passwd",
			want:  "/host/etc/passwd",
		},
		{
			name:  "traversal from deep path collapsed",
			input: "/var/log/../../../../etc/shadow",
			want:  "/host/etc/shadow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeAndResolvePath(tt.input)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSanitizePodLogPath(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		podName   string
		podUID    string
		want      string
		wantError bool
	}{
		{
			name:      "valid components",
			namespace: "default",
			podName:   "my-app-abc123",
			podUID:    "uid-1234",
			want:      "/host/var/log/pods/default_my-app-abc123_uid-1234",
		},
		{
			name:      "invalid namespace with slashes",
			namespace: "../etc",
			podName:   "my-app",
			podUID:    "uid-1234",
			wantError: true,
		},
		{
			name:      "empty pod UID",
			namespace: "default",
			podName:   "my-app",
			podUID:    "",
			wantError: true,
		},
		{
			name:      "invalid pod name with uppercase",
			namespace: "default",
			podName:   "MyApp",
			podUID:    "uid-1234",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizePodLogPath(tt.namespace, tt.podName, tt.podUID)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestClampLineCount(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"zero defaults to 10", 0, defaultLineCount},
		{"negative defaults to 10", -5, defaultLineCount},
		{"within range", 50, 50},
		{"at max", maxLineCount, maxLineCount},
		{"over max clamped", maxLineCount + 100, maxLineCount},
		{"one", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clampLineCount(tt.input))
		})
	}
}

func TestHeadFile(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\n"
	tmpFile := writeTempFile(t, content)

	t.Run("read first 3 lines", func(t *testing.T) {
		lines, count, err := headFile(tmpFile, 3)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
		assert.Equal(t, "line1\nline2\nline3", lines)
	})

	t.Run("read more lines than exist", func(t *testing.T) {
		lines, count, err := headFile(tmpFile, 100)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
		assert.Equal(t, "line1\nline2\nline3\nline4\nline5", lines)
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, _, err := headFile("/no/such/file", 10)
		assert.Error(t, err)
	})
}

func TestTailFile(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\n"
	tmpFile := writeTempFile(t, content)

	t.Run("read last 3 lines", func(t *testing.T) {
		lines, count, err := tailFile(tmpFile, 3)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
		assert.Equal(t, "line3\nline4\nline5", lines)
	})

	t.Run("read last 1 line", func(t *testing.T) {
		lines, count, err := tailFile(tmpFile, 1)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Equal(t, "line5", lines)
	})

	t.Run("read more lines than exist", func(t *testing.T) {
		lines, count, err := tailFile(tmpFile, 100)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
		assert.Equal(t, "line1\nline2\nline3\nline4\nline5", lines)
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, _, err := tailFile("/no/such/file", 10)
		assert.Error(t, err)
	})
}

func TestFindLatestLogFile(t *testing.T) {
	t.Run("picks highest numbered log", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range []string{"0.log", "1.log", "2.log"} {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644))
		}
		got, err := findLatestLogFile(dir)
		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(got, "2.log"))
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		_, err := findLatestLogFile(dir)
		assert.Error(t, err)
	})

	t.Run("non-existent directory", func(t *testing.T) {
		_, err := findLatestLogFile("/no/such/dir")
		assert.Error(t, err)
	})
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "testlog-*.log")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}
