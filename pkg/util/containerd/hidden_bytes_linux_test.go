// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && containerd

package containerd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type hiddenFixture struct {
	files     map[string]int64
	dirs      []string
	whiteouts []string
	opaque    []string
	symlinks  map[string]string
	hardlinks map[string]string
}

func makeHiddenLayers(t *testing.T, fixtures []hiddenFixture) ([]ImageLayer, overlayMetadataReader) {
	t.Helper()
	layers := make([]ImageLayer, len(fixtures))
	attrs := make(map[string]overlayMetadata)
	for i, fixture := range fixtures {
		root := t.TempDir()
		layers[i] = ImageLayer{DiffID: root, Path: root}
		for path, size := range fixture.files {
			full := filepath.Join(root, path)
			require.NoError(t, os.MkdirAll(filepath.Dir(full), 0700))
			file, err := os.Create(full)
			require.NoError(t, err)
			require.NoError(t, file.Truncate(size))
			require.NoError(t, file.Close())
		}
		for _, path := range fixture.dirs {
			require.NoError(t, os.MkdirAll(filepath.Join(root, path), 0700))
		}
		for _, path := range fixture.whiteouts {
			full := filepath.Join(root, path)
			require.NoError(t, os.MkdirAll(filepath.Dir(full), 0700))
			require.NoError(t, os.WriteFile(full, nil, 0600))
			attrs[full] = overlayMetadata{whiteout: true}
		}
		for _, path := range fixture.opaque {
			full := filepath.Join(root, path)
			require.NoError(t, os.MkdirAll(full, 0700))
			attrs[full] = overlayMetadata{opaque: true}
		}
		for path, target := range fixture.symlinks {
			require.NoError(t, os.Symlink(target, filepath.Join(root, path)))
		}
		for path, target := range fixture.hardlinks {
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0700))
			require.NoError(t, os.Link(filepath.Join(root, target), filepath.Join(root, path)))
		}
	}
	return layers, func(path string, _ bool) (overlayMetadata, error) { return attrs[path], nil }
}

func TestHiddenBytesAttribution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		layers []hiddenFixture
		want   []uint64
	}{
		{"clean", []hiddenFixture{{files: map[string]int64{"app": 4096}}, {files: map[string]int64{"cache": 8192}}}, []uint64{0, 0}},
		{"delete", []hiddenFixture{{files: map[string]int64{"cache": 8192}}, {whiteouts: []string{"cache"}}}, []uint64{8192, 0}},
		{"replace", []hiddenFixture{{files: map[string]int64{"app": 4096}}, {files: map[string]int64{"app": 12288}}}, []uint64{4096, 0}},
		{"replace then delete", []hiddenFixture{{files: map[string]int64{"app": 4096}}, {files: map[string]int64{"app": 12288}}, {whiteouts: []string{"app"}}}, []uint64{4096, 12288, 0}},
		{"delete then recreate", []hiddenFixture{{files: map[string]int64{"app": 4096}}, {whiteouts: []string{"app"}}, {files: map[string]int64{"app": 12288}}}, []uint64{4096, 0, 0}},
		{"mixed", []hiddenFixture{{files: map[string]int64{"app": 4096, "cache": 8192}}, {files: map[string]int64{"app": 12288}, whiteouts: []string{"cache"}}}, []uint64{12288, 0}},
		{"opaque keeps new children", []hiddenFixture{{files: map[string]int64{"dir/old": 4096, "dir/again": 8192}}, {files: map[string]int64{"dir/again": 12288}, opaque: []string{"dir"}}}, []uint64{12288, 0}},
		{"opaque root", []hiddenFixture{{files: map[string]int64{"old": 4096}}, {files: map[string]int64{"new": 8192}, opaque: []string{"."}}}, []uint64{4096, 0}},
		{"directory merge", []hiddenFixture{{files: map[string]int64{"dir/old": 4096}}, {files: map[string]int64{"dir/new": 8192}}}, []uint64{0, 0}},
		{"directory delete", []hiddenFixture{{files: map[string]int64{"dir/old": 4096, "dir/sub/file": 8192}}, {whiteouts: []string{"dir"}}}, []uint64{12288, 0}},
		{"directory to file", []hiddenFixture{{files: map[string]int64{"dir/old": 4096}}, {files: map[string]int64{"dir": 8192}}}, []uint64{4096, 0}},
		{"file to directory", []hiddenFixture{{files: map[string]int64{"dir": 4096}}, {files: map[string]int64{"dir/file": 8192}}}, []uint64{4096, 0}},
		{"symlinks not followed", []hiddenFixture{{files: map[string]int64{"app": 4096}}, {symlinks: map[string]string{"app": "/etc/passwd", "outside": "/does/not/exist"}}}, []uint64{4096, 0}},
		{"sparse logical bytes", []hiddenFixture{{files: map[string]int64{"sparse": 1 << 30}}, {whiteouts: []string{"sparse"}}}, []uint64{1 << 30, 0}},
		{"no filesystem additions", []hiddenFixture{{}, {}}, []uint64{0, 0}},
		{"hardlink untouched", []hiddenFixture{{files: map[string]int64{"z": 4096}, hardlinks: map[string]string{"a": "z"}}}, []uint64{0}},
		{"hardlink survivor", []hiddenFixture{{files: map[string]int64{"a": 4096}, hardlinks: map[string]string{"b": "a"}}, {whiteouts: []string{"a"}}}, []uint64{0, 0}},
		{"hardlink all hidden", []hiddenFixture{{files: map[string]int64{"a": 4096}, hardlinks: map[string]string{"b": "a"}}, {whiteouts: []string{"a", "b"}}}, []uint64{4096, 0}},
		{"hardlink sequential deletion", []hiddenFixture{{files: map[string]int64{"a": 4096}, hardlinks: map[string]string{"b": "a"}}, {whiteouts: []string{"a"}}, {whiteouts: []string{"b"}}}, []uint64{4096, 0, 0}},
		{"hardlink replacement survivor", []hiddenFixture{{files: map[string]int64{"a": 4096}, hardlinks: map[string]string{"b": "a"}}, {files: map[string]int64{"a": 12288}}}, []uint64{0, 0}},
		{"hardlink replacement and delete", []hiddenFixture{{files: map[string]int64{"a": 4096}, hardlinks: map[string]string{"b": "a"}}, {files: map[string]int64{"a": 12288}}, {whiteouts: []string{"a", "b"}}}, []uint64{4096, 12288, 0}},
		{"hardlink outside opaque directory", []hiddenFixture{{files: map[string]int64{"dir/a": 4096}, hardlinks: map[string]string{"b": "dir/a"}}, {opaque: []string{"dir"}}}, []uint64{0, 0}},
		{"hardlinks inside opaque directory", []hiddenFixture{{files: map[string]int64{"dir/a": 4096}, hardlinks: map[string]string{"dir/b": "dir/a"}}, {opaque: []string{"dir"}}}, []uint64{4096, 0}},
		{"directory with hardlinks replaced by file", []hiddenFixture{{files: map[string]int64{"dir/a": 4096}, hardlinks: map[string]string{"dir/b": "dir/a"}}, {files: map[string]int64{"dir": 1024}}}, []uint64{4096, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layers, attrs := makeHiddenLayers(t, tc.layers)
			got, err := calculateHiddenBytes(t.Context(), layers, DefaultHiddenBytesLimits(), attrs)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestHiddenBytesFailureReturnsNoPartialCounts(t *testing.T) {
	layers, attrs := makeHiddenLayers(t, []hiddenFixture{{files: map[string]int64{"app": 4096}}, {whiteouts: []string{"app"}}})
	t.Run("entry bound", func(t *testing.T) {
		got, err := calculateHiddenBytes(t.Context(), layers, HiddenBytesLimits{MaxEntries: 3, MaxPathBytes: 1024}, attrs)
		require.ErrorContains(t, err, "entry limit")
		require.Nil(t, got)
	})
	t.Run("current buffer included in path bound", func(t *testing.T) {
		got, err := calculateHiddenBytes(t.Context(), layers, HiddenBytesLimits{MaxEntries: 100, MaxPathBytes: 6}, attrs)
		require.ErrorContains(t, err, "path memory limit")
		require.Nil(t, got)
	})
	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		got, err := calculateHiddenBytes(ctx, layers, DefaultHiddenBytesLimits(), attrs)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, got)
	})
	t.Run("metadata error", func(t *testing.T) {
		got, err := calculateHiddenBytes(t.Context(), layers, DefaultHiddenBytesLimits(), func(string, bool) (overlayMetadata, error) { return overlayMetadata{}, unix.EACCES })
		require.ErrorIs(t, err, unix.EACCES)
		require.Nil(t, got)
	})
	t.Run("missing layer", func(t *testing.T) {
		got, err := calculateHiddenBytes(t.Context(), []ImageLayer{{Path: filepath.Join(t.TempDir(), "absent")}}, DefaultHiddenBytesLimits(), attrs)
		require.Error(t, err)
		require.Nil(t, got)
	})
	t.Run("hardlink outside scanned layer", func(t *testing.T) {
		require.NoError(t, os.Link(filepath.Join(layers[0].Path, "app"), filepath.Join(t.TempDir(), "linked")))
		got, err := calculateHiddenBytes(t.Context(), layers, DefaultHiddenBytesLimits(), attrs)
		require.ErrorContains(t, err, "hardlink group")
		require.Nil(t, got)
	})
}

func TestReadOverlayMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        overlayMetadata
		fail        bool
	}{
		{"opaque", "y", overlayMetadata{opaque: true}, false},
		{"opaque", "x", overlayMetadata{}, false},
		{"opaque", "unexpected", overlayMetadata{}, true},
		{"whiteout", "", overlayMetadata{whiteout: true}, false},
		{"metacopy", "y", overlayMetadata{}, true},
		{"redirect", "/elsewhere", overlayMetadata{}, true},
	} {
		t.Run(tc.name+tc.value, func(t *testing.T) {
			path := t.TempDir()
			err := unix.Setxattr(path, "user.overlay."+tc.name, []byte(tc.value), 0)
			if errors.Is(err, unix.EOPNOTSUPP) {
				t.Skip("test filesystem lacks user xattrs")
			}
			require.NoError(t, err)
			got, err := readOverlayMetadata(path, true)
			if tc.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
}

func TestReadOverlayMetadataIgnoresInactiveNamespace(t *testing.T) {
	for _, prefix := range []string{"user.overlay.", "trusted.overlay."} {
		for _, name := range []string{"opaque", "whiteout", "metacopy", "redirect"} {
			t.Run(prefix+name, func(t *testing.T) {
				path := t.TempDir()
				err := unix.Setxattr(path, prefix+name, []byte("y"), 0)
				if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EPERM) {
					t.Skip("test filesystem or capabilities do not support this xattr namespace")
				}
				require.NoError(t, err)
				// Select the opposite namespace: the attribute must have no effect,
				// including no unsupported-feature error for metacopy or redirect.
				got, err := readOverlayMetadata(path, prefix == "trusted.overlay.")
				require.NoError(t, err)
				require.Equal(t, overlayMetadata{}, got)
				got, err = readOverlayMetadata(path, prefix == "user.overlay.")
				switch name {
				case "opaque":
					require.NoError(t, err)
					require.True(t, got.opaque)
				case "whiteout":
					require.NoError(t, err)
					require.True(t, got.whiteout)
				default:
					require.ErrorContains(t, err, "unsupported overlay "+name)
				}
			})
		}
	}
}

func TestHiddenBytesNativeWhiteout(t *testing.T) {
	layers, attrs := makeHiddenLayers(t, []hiddenFixture{{files: map[string]int64{"app": 4096}}, {}})
	err := unix.Mknod(filepath.Join(layers[1].Path, "app"), unix.S_IFCHR|0600, int(unix.Mkdev(0, 0)))
	if errors.Is(err, unix.EPERM) {
		t.Skip("CAP_MKNOD unavailable")
	}
	require.NoError(t, err)
	got, err := calculateHiddenBytes(t.Context(), layers, DefaultHiddenBytesLimits(), attrs)
	require.NoError(t, err)
	require.Equal(t, []uint64{4096, 0}, got)
}

func TestHiddenBytesReadsNativeUserXattrs(t *testing.T) {
	layers, _ := makeHiddenLayers(t, []hiddenFixture{
		{files: map[string]int64{"dir/old": 4096, "gone": 8192}},
		{files: map[string]int64{"dir/new": 12288, "gone": 0}, symlinks: map[string]string{"outside": "/does/not/exist"}},
	})
	err := unix.Setxattr(filepath.Join(layers[1].Path, "dir"), "user.overlay.opaque", []byte("y"), 0)
	if errors.Is(err, unix.EOPNOTSUPP) {
		t.Skip("test filesystem lacks user xattrs")
	}
	require.NoError(t, err)
	require.NoError(t, unix.Setxattr(filepath.Join(layers[1].Path, "gone"), "user.overlay.whiteout", nil, 0))
	// Inactive user.overlay markers on a trusted-overlay mount do not hide files.
	got, err := calculateHiddenBytes(t.Context(), layers, DefaultHiddenBytesLimits(), readOverlayMetadata)
	require.NoError(t, err)
	// The ordinary zero-byte replacement of gone still hides its old 8192 bytes.
	require.Equal(t, []uint64{8192, 0}, got)
	for i := range layers {
		layers[i].UserXAttr = true
	}
	got, err = calculateHiddenBytes(t.Context(), layers, DefaultHiddenBytesLimits(), readOverlayMetadata)
	require.NoError(t, err)
	require.Equal(t, []uint64{12288, 0}, got)
}

func TestHiddenBytesChecksMetadataPrivilege(t *testing.T) {
	if err := checkOverlayMetadataAccess(); err != nil {
		got, scanErr := CalculateHiddenBytes(t.Context(), nil, DefaultHiddenBytesLimits())
		require.Error(t, scanErr)
		require.Nil(t, got)
	}
}

func TestHiddenBytesInitialUserNamespaceIdentity(t *testing.T) {
	require.True(t, isInitialUserNamespace(4026531837))
	require.False(t, isInitialUserNamespace(0))
	require.False(t, isInitialUserNamespace(4026532999))
}
