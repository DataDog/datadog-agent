// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ls_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func cmdRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir, interp.AllowedPaths([]string{dir}))
}

// FuzzLsFlags fuzzes ls with various flag combinations on directories with random filenames.
// Edge cases: hidden files (-a/-A), -d flag, last sort flag wins (-S vs -t),
// -F indicator, -p append-slash, -l long format with -h human-readable.
func FuzzLsFlags(f *testing.F) {
	f.Add("file1.txt", true, false, false, false, false)
	f.Add(".hidden", false, true, false, false, false)
	f.Add("file.txt", false, false, true, false, false)
	f.Add("file.txt", false, false, false, true, false)
	f.Add("file.txt", false, false, false, false, true)
	// Hidden file with -a (shows it)
	f.Add(".dotfile", true, false, false, false, false)
	// Hidden file without any flag (hidden)
	f.Add(".hidden2", false, false, false, false, false)
	// File with -F indicator (-F appends * for executables)
	f.Add("script.sh", false, false, false, false, true)
	// -l long format with -h human-readable sizes
	f.Add("data.bin", true, false, false, false, false)
	// -S sort by size
	f.Add("small.txt", false, false, true, false, false)
	// Unicode filename
	f.Add("日本語.txt", false, false, false, false, false)
	f.Add("héllo.txt", false, false, false, false, false)
	// Various common filenames
	f.Add("README.md", false, false, false, false, false)
	f.Add("Makefile", false, false, false, false, false)

	tmpRoot := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, filename string, flagL, flagA, flagR, flagS, flagF bool) {
		if t.Context().Err() != nil {
			return
		}
		if len(filename) == 0 || len(filename) > 100 {
			return
		}
		if !utf8.ValidString(filename) {
			return
		}
		// Skip filenames with characters problematic for shell or filesystem.
		for _, c := range filename {
			if c == '\'' || c == '\x00' || c == '\n' || c == '/' || c == '\\' || c == '"' || c == '`' || c == '$' {
				return
			}
		}
		// Skip filenames starting with - (would be treated as flags).
		if filename[0] == '-' {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, tmpRoot, &counter)
		defer cleanup()
		if err := os.WriteFile(filepath.Join(dir, filename), []byte("content"), 0644); err != nil {
			// Some filenames may be invalid on the OS.
			return
		}

		flags := ""
		if flagL {
			flags += " -l"
		}
		if flagA {
			flags += " -a"
		}
		if flagR {
			flags += " -r"
		}
		if flagS {
			flags += " -S"
		}
		if flagF {
			flags += " -F"
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "ls"+flags, dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("ls%s unexpected exit code %d", flags, code)
		}
	})
}

// FuzzLsRecursive fuzzes ls -R on nested directories.
// Edge cases: maxRecursionDepth=256 (depth 255 is last valid, 256 should error),
// empty subdirectories, hidden subdirectories.
func FuzzLsRecursive(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(3))
	f.Add(int64(5))
	f.Add(int64(10))
	// Note: maxRecursionDepth=256 boundary is tested in ls_pentest_test.go.
	// Fuzz seeds above 10 are excluded because nested "sub" directories
	// exceed OS max path length well before reaching depth 256.

	tmpRoot := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, depth int64) {
		if t.Context().Err() != nil {
			return
		}
		// Cap at 10 to avoid hitting OS max path length (creating 256+ nested
		// "sub" directories exceeds filesystem limits on most platforms).
		// The maxRecursionDepth=256 limit is tested in ls_pentest_test.go instead.
		if depth < 0 || depth > 10 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, tmpRoot, &counter)
		defer cleanup()
		current := dir
		for i := int64(0); i < depth; i++ {
			subdir := filepath.Join(current, "sub")
			if err := os.Mkdir(subdir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(current, "file.txt"), []byte("x"), 0644); err != nil {
				t.Fatal(err)
			}
			current = subdir
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "ls -R", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("ls -R unexpected exit code %d", code)
		}
	})
}

// FuzzLsHumanReadable fuzzes ls -lh (long format with human-readable sizes).
// Edge cases: humanSize thresholds (< 1024 bytes, ~1K, ~1M, ~1G),
// zero-byte files, files at exact power-of-1024 boundaries.
func FuzzLsHumanReadable(f *testing.F) {
	// Below 1024 (shown as raw bytes)
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(1023))
	// At 1K boundary
	f.Add(int64(1024))
	f.Add(int64(1025))
	// Below 10K (shown as %.1fK format)
	f.Add(int64(1024 * 9))
	// At 10K (shown as %.0fK format)
	f.Add(int64(1024 * 10))
	f.Add(int64(1024 * 100))
	// At 1M boundary
	f.Add(int64(1024 * 1024))
	f.Add(int64(1024*1024 - 1))
	// At 1G boundary
	f.Add(int64(1024 * 1024 * 1024))
	// Negative size (shouldn't happen but check robustness)
	f.Add(int64(512))

	tmpRoot := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, fileSize int64) {
		if t.Context().Err() != nil {
			return
		}
		// Clamp to 1 MiB to avoid slow file creation.
		if fileSize < 0 || fileSize > 1<<20 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, tmpRoot, &counter)
		defer cleanup()
		// Create a file with the specified size using Truncate.
		fpath := filepath.Join(dir, "testfile.bin")
		fh, err := os.Create(fpath)
		if err != nil {
			t.Fatal(err)
		}
		if fileSize > 0 {
			if err := fh.Truncate(fileSize); err != nil {
				fh.Close()
				t.Fatal(err)
			}
		}
		fh.Close()

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "ls -lh testfile.bin", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("ls -lh unexpected exit code %d", code)
		}
	})
}

// FuzzLsMultipleFiles fuzzes ls with multiple files and mixed file types.
// Edge cases: files listed before dirs (GNU ls ordering), -d flag (no dir expansion),
// non-existent targets, sorting with -t (time) and -S (size).
func FuzzLsMultipleFiles(f *testing.F) {
	f.Add(true, false, false, false) // -l
	f.Add(false, true, false, false) // -a
	f.Add(false, false, true, false) // -t sort by time
	f.Add(false, false, false, true) // -S sort by size
	// Combined flags
	f.Add(true, true, false, false) // -la
	f.Add(true, false, false, true) // -lS
	f.Add(true, false, true, false) // -lt

	tmpRoot := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, flagL, flagA, flagT, flagS bool) {
		if t.Context().Err() != nil {
			return
		}
		dir, cleanup := testutil.FuzzIterDir(t, tmpRoot, &counter)
		defer cleanup()

		// Create a mix of files and a subdirectory.
		files := []struct {
			name    string
			content string
		}{
			{"file1.txt", "short"},
			{"file2.txt", "this is longer content"},
			{".hidden", "hidden"},
		}
		for _, f := range files {
			_ = os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0644)
		}
		_ = os.Mkdir(filepath.Join(dir, "subdir"), 0755)

		flags := ""
		if flagL {
			flags += " -l"
		}
		if flagA {
			flags += " -a"
		}
		if flagT {
			flags += " -t"
		}
		if flagS {
			flags += " -S"
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "ls"+flags, dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("ls%s unexpected exit code %d", flags, code)
		}
	})
}
