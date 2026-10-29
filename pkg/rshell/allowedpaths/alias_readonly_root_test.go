// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aliasFixture is a writable parent root with a nested read-only root
// holding secret.txt.
type aliasFixture struct {
	parent, protected, secret string
}

func newAliasFixture(t *testing.T) aliasFixture {
	t.Helper()
	parent := t.TempDir()
	protected := filepath.Join(parent, "Protected")
	require.NoError(t, os.Mkdir(protected, 0755))
	secret := filepath.Join(protected, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("original"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(parent, "other"), 0755))
	return aliasFixture{parent: parent, protected: protected, secret: secret}
}

// assertReadOnlyRootWritesDenied checks every write primitive against
// target, which names fx.secret through a spelling that lexically falls
// under the writable parent root only.
func assertReadOnlyRootWritesDenied(t *testing.T, sb *Sandbox, fx aliasFixture, target string) {
	t.Helper()

	f, err := sb.Open(target, fx.parent, os.O_WRONLY|os.O_TRUNC, 0)
	if f != nil {
		f.Close()
	}
	assert.ErrorIs(t, err, os.ErrPermission, "Open for write")
	f, err = sb.Open(target, fx.parent, os.O_WRONLY|os.O_APPEND, 0)
	if f != nil {
		f.Close()
	}
	assert.ErrorIs(t, err, os.ErrPermission, "Open for append")
	assert.ErrorIs(t, sb.Truncate(target, fx.parent, 0, false), os.ErrPermission, "Truncate")
	assert.ErrorIs(t, sb.Remove(target, fx.parent), os.ErrPermission, "Remove")
	assert.Error(t, sb.CheckPath(target, fx.parent, PathWrite), "CheckPath write")
	assert.Error(t, sb.CheckPath(target, fx.parent, PathRemove), "CheckPath remove")

	got, err := os.ReadFile(fx.secret)
	require.NoError(t, err)
	assert.Equal(t, "original", string(got), "file under the read-only root must be unchanged")
}

// assertLegitimateAccessAllowed checks that reads, including of the
// read-only root's files, and writes elsewhere in the writable parent
// still work.
func assertLegitimateAccessAllowed(t *testing.T, sb *Sandbox, fx aliasFixture, readTarget string) {
	t.Helper()

	r, err := sb.Open(readTarget, fx.parent, os.O_RDONLY, 0)
	require.NoError(t, err, "read of a file under the read-only root")
	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	require.NoError(t, r.Close())
	assert.Equal(t, "original", string(buf[:n]))

	w, err := sb.Open(filepath.Join("other", "new.txt"), fx.parent, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	require.NoError(t, err, "write elsewhere in the writable parent")
	_, err = w.Write([]byte("ok"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, sb.Truncate(filepath.Join("other", "new.txt"), fx.parent, 1, false))
	require.NoError(t, sb.Remove(filepath.Join("other", "new.txt"), fx.parent))
	assert.NoError(t, sb.CheckPath(filepath.Join("other", "x.txt"), fx.parent, PathWrite))
}

// TestReadOnlyRootAliasSpellingRejected covers a read-only root whose
// configured spelling differs from the spelling a script uses for the same
// directory, as letter case does on a case-insensitive volume or an 8.3
// short name does on NTFS. Lexical root selection then picks the writable
// parent, so the write must be refused by filesystem identity. The alias is
// simulated by giving the read-only root a configured spelling that differs
// from the on-disk name, which works on every filesystem.
func TestReadOnlyRootAliasSpellingRejected(t *testing.T) {
	fx := newAliasFixture(t)
	sb, _, err := New([]string{fx.parent + ":rw", fx.protected + ":ro"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	alias := filepath.Join(fx.parent, "PROTECTED-ALIAS")
	for i := range sb.roots {
		if sb.roots[i].mode == pathModeReadOnly {
			sb.roots[i].absPath = alias
			sb.roots[i].canonicalAbsPath = alias
		}
	}

	t.Run("direct", func(t *testing.T) {
		assertReadOnlyRootWritesDenied(t, sb, fx, filepath.Join("Protected", "secret.txt"))
	})
	t.Run("absolute", func(t *testing.T) {
		assertReadOnlyRootWritesDenied(t, sb, fx, fx.secret)
	})
	t.Run("through in-root symlink", func(t *testing.T) {
		if err := os.Symlink("Protected", filepath.Join(fx.parent, "link")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		assertReadOnlyRootWritesDenied(t, sb, fx, filepath.Join("link", "secret.txt"))
	})
	t.Run("root directory itself", func(t *testing.T) {
		assert.ErrorIs(t, sb.Remove("Protected", fx.parent), os.ErrPermission)
		_, err := os.Stat(fx.protected)
		assert.NoError(t, err)
	})
	t.Run("legitimate access", func(t *testing.T) {
		assertLegitimateAccessAllowed(t, sb, fx, filepath.Join("Protected", "secret.txt"))
	})
}

// caseInsensitiveDir reports whether dir is on a case-insensitive
// filesystem (default macOS APFS, Windows NTFS).
func caseInsensitiveDir(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	require.NoError(t, os.WriteFile(probe, nil, 0644))
	defer os.Remove(probe)
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

// TestReadOnlyRootCaseVariantRejected is the end-to-end form of
// TestReadOnlyRootAliasSpellingRejected on a real case-insensitive
// filesystem. It runs on macOS and Windows and skips on case-sensitive
// volumes.
func TestReadOnlyRootCaseVariantRejected(t *testing.T) {
	fx := newAliasFixture(t)
	if !caseInsensitiveDir(t, fx.parent) {
		t.Skip("temporary directory is on a case-sensitive filesystem")
	}

	t.Run("nested read-only root", func(t *testing.T) {
		sb, _, err := New([]string{fx.parent + ":rw", fx.protected + ":ro"})
		require.NoError(t, err)
		defer sb.Close()
		sb.SetWritable()

		// Correct spelling is denied lexically; case variants must be too.
		assertReadOnlyRootWritesDenied(t, sb, fx, filepath.Join("Protected", "secret.txt"))
		assertReadOnlyRootWritesDenied(t, sb, fx, filepath.Join("protected", "secret.txt"))
		assertReadOnlyRootWritesDenied(t, sb, fx, filepath.Join("PROTECTED", "SECRET.TXT"))
		assertLegitimateAccessAllowed(t, sb, fx, filepath.Join("protected", "secret.txt"))
	})

	t.Run("writable root aliasing a read-only root", func(t *testing.T) {
		// The same directory granted read-only and, under another case
		// spelling, read-write: read-only wins, as for identical spellings.
		variant := filepath.Join(fx.parent, "PROTECTED")
		sb, _, err := New([]string{variant + ":rw", fx.protected + ":ro"})
		require.NoError(t, err)
		defer sb.Close()
		sb.SetWritable()

		f, err := sb.Open(filepath.Join(variant, "secret.txt"), fx.parent, os.O_WRONLY|os.O_TRUNC, 0)
		if f != nil {
			f.Close()
		}
		assert.ErrorIs(t, err, os.ErrPermission)
		got, err := os.ReadFile(fx.secret)
		require.NoError(t, err)
		assert.Equal(t, "original", string(got))
	})
}
