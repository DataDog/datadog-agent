// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestPARConfigMigration(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("migration snapshots must be owned by root")
	}
	for _, kind := range []string{"regular", "empty", "symlink", "dangling"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			config := filepath.Join(base, "config")
			state := filepath.Join(base, "state")
			require.NoError(t, os.Mkdir(state, 0700))
			live := filepath.Join(config, parConfigRelativePath)
			require.NoError(t, os.MkdirAll(filepath.Dir(live), 0755))
			switch kind {
			case "regular":
				require.NoError(t, os.WriteFile(live, []byte("customer bytes\n"), 0400))
			case "empty":
				require.NoError(t, os.WriteFile(live, nil, 0400))
			default:
				target := filepath.Join(base, "target")
				if kind == "symlink" {
					require.NoError(t, os.WriteFile(target, []byte("target bytes"), 0400))
				}
				require.NoError(t, os.Symlink(target, live))
			}
			saved := filepath.Join(state, "script-config.yaml")
			require.NoError(t, unix.Linkat(unix.AT_FDCWD, live, unix.AT_FDCWD, saved, 0))
			original, err := os.Lstat(live)
			require.NoError(t, err)
			migration, err := openPARConfigMigration(state, config)
			require.NoError(t, err)
			require.NotNil(t, migration)
			defer migration.close()
			require.NoError(t, migration.prepare())
			_, err = os.Lstat(live)
			require.ErrorIs(t, err, os.ErrNotExist)
			// Failed restoration must retain the snapshot for retry.
			require.NoError(t, os.Rename(filepath.Dir(live), filepath.Dir(live)+".removed"))
			require.Error(t, migration.restore())
			_, err = os.Lstat(saved)
			require.NoError(t, err)
			require.NoError(t, os.Rename(filepath.Dir(live)+".removed", filepath.Dir(live)))
			require.NoError(t, migration.restore())
			restored, err := os.Lstat(live)
			require.NoError(t, err)
			require.True(t, os.SameFile(original, restored), "preserve the exact inode and its metadata")
			// A post-install retry must detach before recursive ownership setup.
			require.NoError(t, migration.prepare())
			require.NoError(t, migration.restore())
			require.NoError(t, migration.commit())
			_, err = os.Lstat(saved)
			require.ErrorIs(t, err, os.ErrNotExist)
			_, err = os.Lstat(live)
			require.NoError(t, err)
		})
	}
}

func TestPARConfigMigrationRejectsChangedLiveEntry(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("migration snapshots must be owned by root")
	}
	base := t.TempDir()
	state := filepath.Join(base, "state")
	require.NoError(t, os.Mkdir(state, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(state, "script-config.yaml"), []byte("saved"), 0400))
	live := filepath.Join(base, "config", parConfigRelativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(live), 0755))
	require.NoError(t, os.WriteFile(live, []byte("new customer edit"), 0400))
	migration, err := openPARConfigMigration(state, filepath.Join(base, "config"))
	require.NoError(t, err)
	defer migration.close()
	require.ErrorContains(t, migration.prepare(), "changed while migration is pending")
	data, err := os.ReadFile(live)
	require.NoError(t, err)
	require.Equal(t, "new customer edit", string(data))
}

func TestPARConfigMigrationRejectsInsecureState(t *testing.T) {
	base := t.TempDir()
	state := filepath.Join(base, "state")
	require.NoError(t, os.Mkdir(state, 0755))
	_, err := openPARConfigMigration(state, base)
	require.Error(t, err)
	require.NoError(t, os.Remove(state))
	require.NoError(t, os.Symlink(base, state))
	_, err = openPARConfigMigration(state, base)
	require.Error(t, err)
}

func TestPARConfigMigrationMissingState(t *testing.T) {
	migration, err := openPARConfigMigration(filepath.Join(t.TempDir(), "missing"), t.TempDir())
	require.NoError(t, err)
	require.Nil(t, migration)
}
