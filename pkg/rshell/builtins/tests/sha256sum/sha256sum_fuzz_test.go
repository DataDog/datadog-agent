// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sha256sum_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
)

func FuzzSHA256SumContent(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("abc"))
	f.Add([]byte{0, 1, 0xff, '\r', '\n'})
	f.Add([]byte("line one\r\nline two\n"))
	f.Add([]byte{0xc0, 0x80, 0xed, 0xa0, 0x80})
	f.Add([]byte("\x7fELF\x00PK\x03\x04MZ\x1b[31m"))
	f.Add(bytes.Repeat([]byte("a"), 32*1024-1))
	f.Add(bytes.Repeat([]byte("a"), 32*1024))
	f.Add(bytes.Repeat([]byte("a"), 32*1024+1))

	baseDir := f.TempDir()
	var counter atomic.Int64
	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil || len(input) > 1<<20 {
			return
		}
		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()
		if err := os.WriteFile(filepath.Join(dir, "input"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		stdout, stderr, code := shaRunCtx(ctx, t, "sha256sum input", dir, dir)
		if t.Context().Err() != nil {
			return
		}
		if code != 0 {
			t.Fatalf("unexpected exit code %d: %s", code, stderr)
		}
		want := sha256.Sum256(input)
		expected := hex.EncodeToString(want[:]) + "  input\n"
		if stdout != expected || stderr != "" {
			t.Fatalf("got stdout=%q stderr=%q, want stdout=%q", stdout, stderr, expected)
		}
	})
}

func FuzzSHA256SumManifest(f *testing.F) {
	const abcDigest = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	f.Add([]byte(""))
	f.Add([]byte("not a checksum\n"))
	f.Add([]byte(abcDigest + "  target\n"))
	f.Add([]byte(" \t" + abcDigest + "\ttarget\r\n"))
	f.Add([]byte("SHA256 (target) = " + abcDigest + "\r"))
	f.Add([]byte("\\" + abcDigest + "  bad\\q\n"))
	f.Add([]byte{0xff, 0, '\n', '\r'})
	f.Add(bytes.Repeat([]byte("x"), 4*1024-1))
	f.Add(bytes.Repeat([]byte("x"), 4*1024))
	f.Add(bytes.Repeat([]byte("x"), 4*1024+1))
	f.Add(bytes.Repeat([]byte("x"), 64*1024-1))
	f.Add(bytes.Repeat([]byte("x"), 64*1024))
	f.Add(bytes.Repeat([]byte("x"), 64*1024+1))

	baseDir := f.TempDir()
	var counter atomic.Int64
	f.Fuzz(func(t *testing.T, manifest []byte) {
		if t.Context().Err() != nil || len(manifest) > 256*1024 {
			return
		}
		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()
		if err := os.WriteFile(filepath.Join(dir, "target"), []byte("abc"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "checksums"), manifest, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		_, _, code := shaRunCtx(ctx, t, "sha256sum -c --status checksums", dir, dir)
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Fatalf("unexpected exit code %d", code)
		}
	})
}
