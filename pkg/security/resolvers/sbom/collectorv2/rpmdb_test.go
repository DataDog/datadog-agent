// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package collectorv2

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCopyRpmdbFoldsWAL checks that the copy of a SQLite rpm database holds the
// transactions its write-ahead log commits, up to the first invalid frame.
func TestCopyRpmdbFoldsWAL(t *testing.T) {
	const pageSize = 4096
	inserts := []string{
		"INSERT INTO Packages (blob) VALUES (x'02')",
		"INSERT INTO Packages (blob) VALUES (x'03')",
	}
	for _, tc := range []struct {
		name   string
		commit []string // transactions the log keeps
		damage func(wal []byte) []byte
		rows   int
	}{
		{name: "intact", commit: inserts, rows: 4},
		{
			name:   "torn last transaction",
			commit: []string{inserts[0], "INSERT INTO Packages (blob) VALUES (zeroblob(65536))"},
			damage: func(wal []byte) []byte { return wal[:len(wal)-1] },
			rows:   3,
		},
		{
			name:   "corrupt last frame",
			commit: inserts,
			damage: func(wal []byte) []byte { wal[len(wal)-1] ^= 1; return wal },
			rows:   3,
		},
		{
			name:   "last frame from an earlier log",
			commit: inserts,
			damage: func(wal []byte) []byte { wal[len(wal)-pageSize-16] ^= 1; return wal },
			rows:   3,
		},
		{
			name:   "corrupt header",
			commit: inserts,
			damage: func(wal []byte) []byte { wal[24] ^= 1; return wal },
			rows:   2,
		},
		{
			name: "grown then shrunk",
			commit: []string{
				"INSERT INTO Packages (blob) VALUES (zeroblob(65536))",
				"DELETE FROM Packages WHERE length(blob) > 1",
			},
			rows: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "var/lib/rpm/rpmdb.sqlite")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

			writer, err := sql.Open("sqlite3", path)
			require.NoError(t, err)
			_, err = writer.Exec(`PRAGMA page_size=4096;
				PRAGMA auto_vacuum=FULL;
				PRAGMA journal_mode=WAL;
				PRAGMA wal_autocheckpoint=0;
				CREATE TABLE Packages (hnum INTEGER PRIMARY KEY, blob BLOB);
				INSERT INTO Packages (blob) VALUES (x'01'), (zeroblob(65536));
				PRAGMA wal_checkpoint(TRUNCATE);`)
			require.NoError(t, err)

			// A reader keeps the writer from folding the log in when it closes.
			reader, err := sql.Open("sqlite3", path)
			require.NoError(t, err)
			conn, err := reader.Conn(context.Background())
			require.NoError(t, err)
			t.Cleanup(func() { conn.Close(); reader.Close() })
			var n int
			require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT count(*) FROM Packages").Scan(&n))

			for _, stmt := range tc.commit {
				_, err = writer.Exec(stmt)
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			if tc.damage != nil {
				wal, err := os.ReadFile(path + "-wal")
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path+"-wal", tc.damage(wal), 0o644))
			}

			root, err := os.OpenRoot(dir)
			require.NoError(t, err)
			defer root.Close()
			tempDir, db, err := copyRpmdb(root, "var/lib/rpm/rpmdb.sqlite")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// rpmdb.Open opens the copy this way.
			copied, err := sql.Open("sqlite3", "file:"+db+"?mode=ro&immutable=1")
			require.NoError(t, err)
			defer copied.Close()
			var check string
			require.NoError(t, copied.QueryRow("PRAGMA integrity_check").Scan(&check))
			assert.Equal(t, "ok", check)
			require.NoError(t, copied.QueryRow("SELECT count(*) FROM Packages").Scan(&n))
			assert.Equal(t, tc.rows, n, "the copy holds other transactions than the log commits")
			var pages int64
			require.NoError(t, copied.QueryRow("PRAGMA page_count").Scan(&pages))
			info, err := os.Stat(db)
			require.NoError(t, err)
			assert.Equal(t, pages*pageSize, info.Size(), "the copy holds pages past the end of the database")

			_, err = os.Stat(path + "-wal")
			assert.NoError(t, err, "the copy changed the source database")
		})
	}
}

// TestCopyRpmdbCopiesAgainWhileWritten checks that an rpm database written
// during its copy is copied again, and given up on when written at every copy.
func TestCopyRpmdbCopiesAgainWhileWritten(t *testing.T) {
	for _, tc := range []struct {
		name   string
		writes int // writes to the database, one after each copy
		copied bool
	}{
		{name: "written once", writes: 1, copied: true},
		{name: "written at every copy", writes: maxRpmdbCopies},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "var/lib/rpm/Packages")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte("rpmdb"), 0o644))

			content := "rpmdb"
			writes := 0
			testHookRpmdbCopied = func() {
				if writes < tc.writes {
					writes++
					content += "+"
					require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
				}
			}
			t.Cleanup(func() { testHookRpmdbCopied = func() {} })

			root, err := os.OpenRoot(dir)
			require.NoError(t, err)
			defer root.Close()
			tempDir, db, err := copyRpmdb(root, "var/lib/rpm/Packages")
			if !tc.copied {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			got, err := os.ReadFile(db)
			require.NoError(t, err)
			assert.Equal(t, content, string(got), "the copy holds a state the database left")
		})
	}
}

// TestFingerprintFollowsRpmdbWAL checks that a transaction rpm commits to the
// write-ahead log of its database changes the fingerprint.
func TestFingerprintFollowsRpmdbWAL(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "var/lib/rpm/rpmdb.sqlite")
	require.NoError(t, os.MkdirAll(filepath.Dir(db), 0o755))
	require.NoError(t, os.WriteFile(db, []byte("rpmdb"), 0o644))
	require.NoError(t, os.WriteFile(db+"-wal", []byte("frame"), 0o644))
	scanner := NewOSScanner()

	before := scanner.Fingerprint(dir)
	require.NoError(t, os.WriteFile(db+"-wal", []byte("frame frame"), 0o644))
	assert.NotEqual(t, before, scanner.Fingerprint(dir), "a commit to the log kept the fingerprint")
}
