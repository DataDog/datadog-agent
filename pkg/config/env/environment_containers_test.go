// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_merge(t *testing.T) {
	tests := []struct {
		name string
		s1   []string
		s2   []string
		want []string
	}{
		{
			name: "nominal case",
			s1:   []string{"foo", "bar"},
			s2:   []string{"baz", "tar"},
			want: []string{"foo", "bar", "baz", "tar"},
		},
		{
			name: "empty s1",
			s1:   []string{},
			s2:   []string{"baz", "tar"},
			want: []string{"baz", "tar"},
		},
		{
			name: "empty s2",
			s1:   []string{"foo", "bar"},
			s2:   []string{},
			want: []string{"foo", "bar"},
		},
		{
			name: "dedupe 1",
			s1:   []string{"foo", "bar"},
			s2:   []string{"baz", "foo"},
			want: []string{"foo", "bar", "baz"},
		},
		{
			name: "dedupe 2",
			s1:   []string{"foo", "foo"},
			s2:   []string{"foo", "foo"},
			want: []string{"foo"},
		},
		{
			name: "dedupe 3",
			s1:   []string{"foo", "foo"},
			s2:   []string{"baz", "tar"},
			want: []string{"foo", "baz", "tar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualValues(t, tt.want, merge(tt.s1, tt.s2))
		})
	}
}

func TestDetectPodmanInHomeDir(t *testing.T) {
	tmp := t.TempDir()

	t.Run("detects podman when storage dir exists under home", func(t *testing.T) {
		userStoragePath := filepath.Join(tmp, "testuser", ".local", "share", "containers", "storage")
		require.NoError(t, os.MkdirAll(userStoragePath, 0o755))

		features := make(FeatureMap)
		detectPodmanInHomeDir(tmp, features)

		_, found := features[Podman]
		assert.True(t, found, "Podman feature should be detected")
	})

	t.Run("no detection when storage dir does not exist", func(t *testing.T) {
		emptyDir := filepath.Join(tmp, "empty")
		require.NoError(t, os.MkdirAll(emptyDir, 0o755))

		features := make(FeatureMap)
		detectPodmanInHomeDir(emptyDir, features)

		_, found := features[Podman]
		assert.False(t, found, "Podman feature should not be detected")
	})

	t.Run("no detection when home base does not exist", func(t *testing.T) {
		features := make(FeatureMap)
		detectPodmanInHomeDir(filepath.Join(tmp, "nonexistent"), features)

		_, found := features[Podman]
		assert.False(t, found, "Podman feature should not be detected")
	})
}
