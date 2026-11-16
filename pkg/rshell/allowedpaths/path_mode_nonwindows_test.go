// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package allowedpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Paths can contain colons on POSIX, but a terminal ":ro" or ":rw" is always
// an access mode. An existing entry with the literal suffixed name is never
// opened as a root and never changes the configured root's mode.
func TestAllowedPathSuffixIgnoresExistingLiteralPath(t *testing.T) {
	creators := map[string]func(string) error{
		"file": func(p string) error { return os.WriteFile(p, []byte("literal"), 0644) },
		"dir":  func(p string) error { return os.Mkdir(p, 0755) },
	}
	for _, suffix := range []struct {
		text string
		mode pathMode
	}{{":ro", pathModeReadOnly}, {":rw", pathModeReadWrite}} {
		for name, create := range creators {
			t.Run(suffix.text+"/"+name, func(t *testing.T) {
				base := filepath.Join(t.TempDir(), "tenant")
				literal := base + suffix.text
				require.NoError(t, os.Mkdir(base, 0755))
				require.NoError(t, create(literal))

				sb, warnings, err := New([]string{literal})
				require.NoError(t, err)
				defer sb.Close()
				assert.Equal(t, fmt.Sprintf("AllowedPaths: existing path %q is not granted; the %q suffix is read as the access mode for %q (paths ending in :ro or :rw cannot be allowed)\n",
					literal, suffix.text, base), string(warnings))
				assert.Equal(t, []string{base}, sb.Paths())

				root, _, ok := sb.resolve(filepath.Join(base, "file"))
				require.True(t, ok)
				assert.Equal(t, suffix.mode, root.mode)

				_, err = sb.Stat(literal, "/")
				assert.ErrorIs(t, err, os.ErrPermission, "the literal suffixed path is outside every root")
			})
		}
	}
}

// A script with write access to a read-write root must not be able to drop a
// nested read-only root from later sandboxes by creating "<child>:ro" in it.
func TestNestedReadOnlyRootSurvivesLiteralROSuffixEntry(t *testing.T) {
	for name, create := range map[string]func(string) error{
		"file": func(p string) error { return os.WriteFile(p, []byte("x"), 0644) },
		"dir":  func(p string) error { return os.Mkdir(p, 0755) },
	} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			child := filepath.Join(parent, "protected")
			require.NoError(t, os.Mkdir(child, 0755))
			config := filepath.Join(child, "config")
			key := filepath.Join(child, "key")
			require.NoError(t, os.WriteFile(config, []byte("config"), 0644))
			require.NoError(t, os.WriteFile(key, []byte("key"), 0644))
			require.NoError(t, create(child+":ro"))

			sb, warnings, err := New([]string{parent + ":rw", child + ":ro"})
			require.NoError(t, err)
			defer sb.Close()
			sb.SetWritable()
			assert.Contains(t, string(warnings), fmt.Sprintf("existing path %q is not granted", child+":ro"))
			assert.NotContains(t, string(warnings), "skipping")
			assert.Equal(t, []string{parent, child}, sb.Paths())

			root, _, ok := sb.resolve(config)
			require.True(t, ok)
			assert.Equal(t, child, root.absPath)
			assert.Equal(t, pathModeReadOnly, root.mode)

			f, err := sb.Open(config, "/", os.O_WRONLY|os.O_TRUNC, 0)
			assert.Nil(t, f)
			assert.ErrorIs(t, err, os.ErrPermission)
			assert.ErrorIs(t, sb.Remove(key, "/"), os.ErrPermission)

			// Reads under the read-only root and writes to the parent still work.
			f, err = sb.Open(config, "/", os.O_RDONLY, 0)
			require.NoError(t, err)
			require.NoError(t, f.Close())
			f, err = sb.Open(filepath.Join(parent, "parent.txt"), "/", os.O_WRONLY|os.O_CREATE, 0644)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			got, err := os.ReadFile(config)
			require.NoError(t, err)
			assert.Equal(t, "config", string(got))
			_, err = os.Stat(key)
			assert.NoError(t, err)
		})
	}
}

func TestAllowedPathSuffixWithoutLiteralPathDoesNotWarn(t *testing.T) {
	dir := t.TempDir()
	sb, warnings, err := New([]string{dir + ":rw", dir + ":ro", dir})
	require.NoError(t, err)
	defer sb.Close()
	assert.Empty(t, string(warnings))
}
