// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package walker holds the trivy walkers
package walker

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/walker"
)

func TestFS_Walk(t *testing.T) {
	tests := []struct {
		name      string
		option    walker.Option
		rootDir   string
		analyzeFn walker.WalkFunc
		wantErr   string
	}{
		{
			name:    "happy path",
			rootDir: "testdata/fs",
			analyzeFn: func(_ context.Context, filePath string, _ os.FileInfo, opener analyzer.Opener) error {
				if filePath == "bar" {
					got, err := opener()
					require.NoError(t, err)

					b, err := io.ReadAll(got)
					require.NoError(t, err)

					assert.Equal(t, "bar\n", string(b))
				}
				return nil
			},
		},
		{
			name:    "skip file",
			rootDir: "testdata/fs",
			option: walker.Option{
				SkipFiles: []string{"bar"},
			},
			analyzeFn: func(_ context.Context, filePath string, _ os.FileInfo, _ analyzer.Opener) error {
				if filePath == "bar" {
					assert.Fail(t, "skip files error", "%s should be skipped", filePath)
				}
				return nil
			},
		},
		{
			name:    "skip dir",
			rootDir: "testdata/fs/",
			option: walker.Option{
				SkipDirs: []string{"/app"},
			},
			analyzeFn: func(_ context.Context, filePath string, _ os.FileInfo, _ analyzer.Opener) error {
				if strings.Contains(filePath, "app") {
					assert.Fail(t, "skip dirs error", "%s should be skipped", filePath)
				}
				return nil
			},
		},
		{
			name:    "sad path",
			rootDir: "testdata/fs",
			analyzeFn: func(context.Context, string, os.FileInfo, analyzer.Opener) error {
				return errors.New("error")
			},
			wantErr: "failed to analyze file",
		},
		{
			name:    "single file root",
			rootDir: "testdata/fs/bar",
			analyzeFn: func(_ context.Context, filePath string, _ os.FileInfo, opener analyzer.Opener) error {
				assert.Equal(t, ".", filePath)
				got, err := opener()
				require.NoError(t, err)
				b, err := io.ReadAll(got)
				require.NoError(t, err)
				assert.Equal(t, "bar\n", string(b))
				return nil
			},
		},
		{
			name:      "missing root",
			rootDir:   "testdata/fs/does-not-exist",
			analyzeFn: func(context.Context, string, os.FileInfo, analyzer.Opener) error { return nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewFSWalker()
			err := w.Walk(context.TODO(), tt.rootDir, tt.option, tt.analyzeFn)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestFS_Walk_SymlinkedRoot ensures a root path that is a symlink still
// resolves to its target: symlink-to-dir is traversed, symlink-to-file is
// analyzed.
func TestFS_Walk_SymlinkedRoot(t *testing.T) {
	fsAbs, err := filepath.Abs("testdata/fs")
	require.NoError(t, err)
	tmp := t.TempDir()
	dirLink := tmp + "/fs-link"
	fileLink := tmp + "/bar-link"
	require.NoError(t, os.Symlink(fsAbs, dirLink))
	require.NoError(t, os.Symlink(fsAbs+"/bar", fileLink))

	t.Run("symlink to dir", func(t *testing.T) {
		var visited []string
		fn := func(_ context.Context, filePath string, _ os.FileInfo, _ analyzer.Opener) error {
			visited = append(visited, filePath)
			return nil
		}
		require.NoError(t, NewFSWalker().Walk(context.TODO(), dirLink, walker.Option{}, fn))
		assert.Contains(t, visited, "bar", "symlinked-dir root should be traversed")
	})

	t.Run("symlink to file", func(t *testing.T) {
		var got []byte
		fn := func(_ context.Context, filePath string, _ os.FileInfo, opener analyzer.Opener) error {
			assert.Equal(t, ".", filePath)
			r, err := opener()
			require.NoError(t, err)
			got, err = io.ReadAll(r)
			require.NoError(t, err)
			return nil
		}
		require.NoError(t, NewFSWalker().Walk(context.TODO(), fileLink, walker.Option{}, fn))
		assert.Equal(t, "bar\n", string(got), "symlinked-file root should be analyzed")
	})
}
