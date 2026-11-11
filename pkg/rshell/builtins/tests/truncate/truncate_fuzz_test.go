// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package truncate_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// FuzzTruncateSize fuzzes "truncate -s SIZE file" with arbitrary SIZE strings.
// The fuzzer should verify: no panic, no hang, exit code is 0 or 1, and
// parseSize rejects/accepts inputs consistently with no memory blowup.
func FuzzTruncateSize(f *testing.F) {
	// Valid bare integers
	f.Add("0")
	f.Add("1")
	f.Add("1024")
	f.Add("9223372036854775807") // MaxInt64
	// Valid suffixes
	f.Add("1K")
	f.Add("1k")
	f.Add("1KiB")
	f.Add("1kiB")
	f.Add("1KB")
	f.Add("1kB")
	f.Add("1M")
	f.Add("1G")
	f.Add("1T")
	f.Add("1P")
	f.Add("1E")
	// Overflow
	f.Add("9223372036854775808")
	f.Add("8E")
	f.Add("8388608T")
	// Relative-size modifiers (all rejected)
	f.Add("+1K")
	f.Add("-1K")
	f.Add("<1K")
	f.Add(">1K")
	f.Add("/1K")
	f.Add("%1K")
	// Invalid
	f.Add("")
	f.Add("abc")
	f.Add("1.5K")
	f.Add("1Z")
	f.Add("1Y")
	f.Add("1p") // lowercase P rejected
	f.Add("1e") // lowercase E rejected

	f.Fuzz(func(t *testing.T, sizeStr string) {
		// Reject inputs that would take too long to parse or are too long.
		if len(sizeStr) > 64 {
			return
		}
		// parseSize only accepts ASCII digits and letter suffixes. Skip inputs
		// with non-ASCII bytes or single quotes (which break shell quoting).
		for i := 0; i < len(sizeStr); i++ {
			b := sizeStr[i]
			if b > 0x7e || b < 0x20 || b == '\'' {
				return
			}
		}

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("seed data"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// Quote the size string to prevent shell metacharacters from being
		// interpreted. Any metacharacter in sizeStr makes the script invalid
		// shell; that's fine — we just want no panic/hang.
		script := fmt.Sprintf("truncate -s '%s' f.txt", sizeStr)
		_, _, code := runScriptCtx(ctx, t, script, dir,
			interp.AllowedPaths([]string{dir + ":rw"}),
			interp.WithMode(interp.ModeRemediation),
		)
		if ctx.Err() != nil {
			t.Fatal("truncate fuzz timed out")
		}
		// Only valid exit codes are 0 or 1.
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d for size %q", code, sizeStr)
		}
	})
}

// FuzzTruncateFileContent fuzzes "truncate -s SIZE file" against files with
// arbitrary content, verifying that file-content does not affect size logic.
func FuzzTruncateFileContent(f *testing.F) {
	f.Add([]byte(""), int64(0))
	f.Add([]byte("hello"), int64(0))
	f.Add([]byte("hello"), int64(10))
	f.Add([]byte("hello world\n"), int64(5))
	f.Add([]byte{0x00, 0x01, 0x02}, int64(1))
	f.Add([]byte("\n\n\n"), int64(1))
	// Binary / high bytes
	f.Add([]byte{0xff, 0xfe, 0xfd}, int64(2))
	// Near-empty cases
	f.Add([]byte("x"), int64(0))
	f.Add([]byte("x"), int64(1))
	f.Add([]byte("x"), int64(2))

	f.Fuzz(func(t *testing.T, content []byte, size int64) {
		// Limit content size and size value to keep tests fast.
		if len(content) > 4096 {
			return
		}
		if size < 0 || size > 1024*1024 {
			return
		}

		dir := t.TempDir()
		fpath := filepath.Join(dir, "f.txt")
		if err := os.WriteFile(fpath, content, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		script := fmt.Sprintf("truncate -s %d f.txt", size)
		_, _, code := runScriptCtx(ctx, t, script, dir,
			interp.AllowedPaths([]string{dir + ":rw"}),
			interp.WithMode(interp.ModeRemediation),
		)
		if ctx.Err() != nil {
			t.Fatal("truncate fuzz timed out")
		}
		if code != 0 {
			t.Errorf("truncate -s %d on %d-byte file failed unexpectedly", size, len(content))
			return
		}
		// After successful truncate, file must be exactly `size` bytes.
		info, err := os.Stat(fpath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != size {
			t.Errorf("expected file size %d, got %d", size, info.Size())
		}
	})
}
