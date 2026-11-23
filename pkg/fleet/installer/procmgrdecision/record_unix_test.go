// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package procmgrdecision

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// newTestStore returns a store in a fresh directory owned by the current user, so no test needs root.
func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "store")
	require.NoError(t, os.Mkdir(dir, 0o755))
	return newStore(dir, os.Getuid(), os.Getgid()), dir
}

func recordPath(dir string) string {
	return filepath.Join(dir, recordName)
}

func modeOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	fi, err := os.Lstat(path)
	require.NoError(t, err)
	return fi.Mode().Perm()
}

func TestDefaultStoreUnix(t *testing.T) {
	s := DefaultStore()
	assert.Equal(t, filepath.Join(paths.PackagesPath, "process_manager"), s.Path())
	assert.Equal(t, 0, s.owner.uid, "the production record must be owned by root")
	assert.Equal(t, 0, s.owner.gid, "the production record directory must be in group 0")
}

func TestStoreReadMissingDirectory(t *testing.T) {
	s := newStore(filepath.Join(t.TempDir(), "missing"), os.Getuid(), os.Getgid())

	enabled, found, err := s.Read()

	require.NoError(t, err)
	assert.False(t, enabled)
	assert.False(t, found)
}

func TestStoreReadMissingFile(t *testing.T) {
	s, _ := newTestStore(t)

	enabled, found, err := s.Read()

	require.NoError(t, err)
	assert.False(t, enabled)
	assert.False(t, found)
}

func TestStoreRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		content string
	}{
		{name: "enabled", enabled: true, content: "v1 enabled\n"},
		{name: "disabled", enabled: false, content: "v1 disabled\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, dir := newTestStore(t)

			require.NoError(t, s.Write(context.Background(), tt.enabled))

			raw, err := os.ReadFile(recordPath(dir))
			require.NoError(t, err)
			assert.Equal(t, tt.content, string(raw))
			assert.Equal(t, fs.FileMode(0o644), modeOf(t, recordPath(dir)))

			enabled, found, err := s.Read()
			require.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, tt.enabled, enabled)
		})
	}
}

func TestStoreWriteCreatesMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	s := newStore(dir, os.Getuid(), os.Getgid())

	require.NoError(t, s.Write(context.Background(), true))

	assert.Equal(t, fs.FileMode(0o755), modeOf(t, dir))
	enabled, found, err := s.Read()
	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, enabled)
}

func TestStoreWriteMissingParentIsNotUntrusted(t *testing.T) {
	s := newStore(filepath.Join(t.TempDir(), "absent", "store"), os.Getuid(), os.Getgid())

	err := s.Write(context.Background(), true)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrUntrusted)
}

func TestStoreWriteRepairsDirectoryMode(t *testing.T) {
	tests := []struct {
		name string
		mode fs.FileMode
		want fs.FileMode
	}{
		{name: "group writable", mode: 0o775, want: 0o755},
		{name: "world writable", mode: 0o757, want: 0o755},
		{name: "fully open", mode: 0o777, want: 0o755},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, dir := newTestStore(t)
			require.NoError(t, os.Chmod(dir, tt.mode))

			require.NoError(t, s.Write(context.Background(), false))

			assert.Equal(t, tt.want, modeOf(t, dir))
			enabled, found, err := s.Read()
			require.NoError(t, err)
			assert.True(t, found)
			assert.False(t, enabled)
		})
	}
}

func TestStoreWriteRefusesDirectorySymlink(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	require.NoError(t, os.Mkdir(real, 0o755))
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(real, link))
	s := newStore(link, os.Getuid(), os.Getgid())

	err := s.Write(context.Background(), true)

	require.ErrorIs(t, err, ErrUntrusted)
	entries, err := os.ReadDir(real)
	require.NoError(t, err)
	assert.Empty(t, entries, "the symlink target must not be written through")
}

func TestStoreWriteRefusesDirectoryThatIsAFile(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "store")
	require.NoError(t, os.WriteFile(dir, []byte("x"), 0o644))
	s := newStore(dir, os.Getuid(), os.Getgid())

	require.ErrorIs(t, s.Write(context.Background(), true), ErrUntrusted)
	_, _, err := s.Read()
	require.ErrorIs(t, err, ErrUntrusted)
}

func TestStoreReadUntrusted(t *testing.T) {
	validRecord := func(t *testing.T, s *Store) {
		t.Helper()
		require.NoError(t, s.Write(context.Background(), true))
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, s *Store, dir string)
	}{
		{
			name: "directory group writable",
			setup: func(t *testing.T, s *Store, dir string) {
				validRecord(t, s)
				require.NoError(t, os.Chmod(dir, 0o775))
			},
		},
		{
			name: "directory world writable",
			setup: func(t *testing.T, s *Store, dir string) {
				validRecord(t, s)
				require.NoError(t, os.Chmod(dir, 0o757))
			},
		},
		{
			name: "directory group writable without a record",
			setup: func(t *testing.T, _ *Store, dir string) {
				require.NoError(t, os.Chmod(dir, 0o775))
			},
		},
		{
			name: "directory world writable without a record",
			setup: func(t *testing.T, _ *Store, dir string) {
				require.NoError(t, os.Chmod(dir, 0o757))
			},
		},
		{
			name: "file group writable",
			setup: func(t *testing.T, s *Store, dir string) {
				validRecord(t, s)
				require.NoError(t, os.Chmod(recordPath(dir), 0o664))
			},
		},
		{
			name: "file world writable",
			setup: func(t *testing.T, s *Store, dir string) {
				validRecord(t, s)
				require.NoError(t, os.Chmod(recordPath(dir), 0o646))
			},
		},
		{
			name: "file is a symlink to a valid record",
			setup: func(t *testing.T, _ *Store, dir string) {
				target := filepath.Join(filepath.Dir(dir), "target")
				require.NoError(t, os.WriteFile(target, []byte("v1 enabled\n"), 0o644))
				require.NoError(t, os.Symlink(target, recordPath(dir)))
			},
		},
		{
			name: "file is a dangling symlink",
			setup: func(t *testing.T, _ *Store, dir string) {
				require.NoError(t, os.Symlink(filepath.Join(dir, "nowhere"), recordPath(dir)))
			},
		},
		{
			name: "directory is a symlink to a valid directory",
			setup: func(t *testing.T, _ *Store, dir string) {
				// Replace the store directory with a symlink to a directory that holds a valid record.
				real := filepath.Join(filepath.Dir(dir), "real")
				require.NoError(t, os.Mkdir(real, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(real, recordName), []byte("v1 enabled\n"), 0o644))
				require.NoError(t, os.Remove(dir)) // removes the directory created by newTestStore
				require.NoError(t, os.Symlink(real, dir))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, dir := newTestStore(t)
			tt.setup(t, s, dir)

			_, found, err := s.Read()

			require.ErrorIs(t, err, ErrUntrusted)
			assert.False(t, found)
		})
	}
}

func TestStoreReadDirectoryOwnedByOtherUID(t *testing.T) {
	_, dir := newTestStore(t)
	s := newStore(dir, os.Getuid()+1, os.Getgid())
	require.NoError(t, newStore(dir, os.Getuid(), os.Getgid()).Write(context.Background(), true))

	_, _, err := s.Read()

	require.ErrorIs(t, err, ErrUntrusted)
}

func TestStoreReadFIFONotBlocking(t *testing.T) {
	s, dir := newTestStore(t)
	require.NoError(t, syscall.Mkfifo(recordPath(dir), 0o644))

	done := make(chan error, 1)
	go func() {
		_, _, err := s.Read()
		done <- err
	}()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, ErrUntrusted)
	case <-time.After(10 * time.Second):
		t.Fatal("reading a FIFO record blocked")
	}
}

func TestStoreReadInvalidRecord(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "empty", content: ""},
		{name: "version only", content: "v1"},
		{name: "other version", content: "v2 enabled\n"},
		{name: "other value", content: "v1 maybe\n"},
		{name: "extra token", content: "v1 enabled extra\n"},
		{name: "two lines", content: "v1 enabled\nv1 enabled\n"},
		{name: "leading space", content: " v1 enabled\n"},
		{name: "crlf", content: "v1 enabled\r\n"},
		{name: "65 bytes", content: string(make([]byte, 65))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, dir := newTestStore(t)
			require.NoError(t, os.WriteFile(recordPath(dir), []byte(tt.content), 0o644))

			_, found, err := s.Read()

			require.ErrorIs(t, err, ErrInvalid)
			assert.False(t, found)
		})
	}
}

func TestStoreWriteReplacesInvalidRecord(t *testing.T) {
	s, dir := newTestStore(t)
	require.NoError(t, os.WriteFile(recordPath(dir), []byte("garbage"), 0o644))

	require.NoError(t, s.Write(context.Background(), true))

	raw, err := os.ReadFile(recordPath(dir))
	require.NoError(t, err)
	assert.Equal(t, "v1 enabled\n", string(raw))
}

func TestStoreWriteReplacesUntrustedRecord(t *testing.T) {
	s, dir := newTestStore(t)
	require.NoError(t, s.Write(context.Background(), true))
	require.NoError(t, os.Chmod(recordPath(dir), 0o666))

	require.NoError(t, s.Write(context.Background(), false))

	assert.Equal(t, fs.FileMode(0o644), modeOf(t, recordPath(dir)))
	enabled, found, err := s.Read()
	require.NoError(t, err)
	assert.True(t, found)
	assert.False(t, enabled)
}

func TestStoreWriteReplacesSymlinkWithoutTouchingTarget(t *testing.T) {
	s, dir := newTestStore(t)
	target := filepath.Join(filepath.Dir(dir), "target")
	require.NoError(t, os.WriteFile(target, []byte("untouched"), 0o644))
	require.NoError(t, os.Symlink(target, recordPath(dir)))

	require.NoError(t, s.Write(context.Background(), true))

	fi, err := os.Lstat(recordPath(dir))
	require.NoError(t, err)
	assert.True(t, fi.Mode().IsRegular(), "the link must be replaced by a regular file")
	raw, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "untouched", string(raw), "the link target must not be written through")
}

func TestStoreWriteIdenticalValueDoesNotRewrite(t *testing.T) {
	s, dir := newTestStore(t)
	require.NoError(t, s.Write(context.Background(), true))
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(recordPath(dir), past, past))

	require.NoError(t, s.Write(context.Background(), true))

	fi, err := os.Stat(recordPath(dir))
	require.NoError(t, err)
	assert.True(t, fi.ModTime().Equal(past), "an identical write must not touch the file")
}

func TestStoreWriteAtomicFailureKeepsOldContent(t *testing.T) {
	s, dir := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.Write(ctx, true))

	original := renameRecord
	t.Cleanup(func() { renameRecord = original })
	renameRecord = func(_ context.Context, _, _ string) error {
		return errors.New("injected rename failure")
	}

	err := s.Write(ctx, false)

	require.Error(t, err)
	enabled, found, readErr := s.Read()
	require.NoError(t, readErr)
	assert.True(t, found)
	assert.True(t, enabled, "the previous record must be intact")
	leftovers, globErr := filepath.Glob(filepath.Join(dir, ".process_manager.*"))
	require.NoError(t, globErr)
	assert.Empty(t, leftovers, "the temporary file must be removed")
}

func TestStoreConcurrentWritersAndReaders(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.Write(ctx, true))

	const writers, readers, rounds = 3, 3, 40
	errs := make(chan error, writers*rounds+readers*rounds*10)
	stop := make(chan struct{})

	var readersWG sync.WaitGroup
	for r := 0; r < readers; r++ {
		readersWG.Add(1)
		go func() {
			defer readersWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, _, err := s.Read(); err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	var writersWG sync.WaitGroup
	for w := 0; w < writers; w++ {
		writersWG.Add(1)
		go func(w int) {
			defer writersWG.Done()
			for i := 0; i < rounds; i++ {
				if err := s.Write(ctx, (i+w)%2 == 0); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	writersWG.Wait()
	close(stop)
	readersWG.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent access failed: %v", err)
	}
}
