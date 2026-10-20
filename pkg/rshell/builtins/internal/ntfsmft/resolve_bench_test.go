// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsmft

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// realCandidates builds fileCandidate values for the files in dir by opening
// each and reading its NTFS file reference (MFT record + sequence) from
// GetFileInformationByHandle — the same 64-bit identity resolveCandidatePaths
// feeds to OpenFileById. Bounded to at most max files. Unlike the raw-$MFT
// scan, this needs no elevation, so the resolve benchmarks run anywhere.
func realCandidates(tb testing.TB, dir string, max int) []fileCandidate {
	tb.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		tb.Skipf("ReadDir(%s): %v", dir, err)
	}
	cands := make([]fileCandidate, 0, max)
	for _, e := range entries {
		if e.IsDir() || len(cands) >= max {
			continue
		}
		p := filepath.Join(dir, e.Name())
		pw, err := windows.UTF16PtrFromString(p)
		if err != nil {
			continue
		}
		h, err := windows.CreateFile(pw,
			windows.FILE_READ_ATTRIBUTES,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING,
			windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			continue
		}
		var info windows.ByHandleFileInformation
		err = windows.GetFileInformationByHandle(h, &info)
		windows.CloseHandle(h)
		if err != nil {
			continue
		}
		ref := uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
		cands = append(cands, fileCandidate{
			idx:      ref & 0x0000FFFFFFFFFFFF,
			sequence: uint16(ref >> 48),
			size:     int64(uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)),
			basename: e.Name(),
		})
	}
	return cands
}

func benchResolveDir() string {
	if d := os.Getenv("RSHELL_NTFSDU_BENCH_TARGET"); d != "" {
		return d
	}
	return `C:\Windows\System32`
}

// BenchmarkResolveCandidatePaths measures the full post-scan per-displayed-file
// resolution as it runs in production — OpenFileById + GetFileInformationByHandle
// (the timestamp query) + GetFinalPathNameByHandle + CloseHandle — over a real
// set of files. Divide ns/op by the reported files/op for the per-file cost.
//
//	go test -run '^$' -bench '^BenchmarkResolve' -benchmem \
//	    -cpuprofile cpu.prof ./builtins/internal/ntfsmft/
func BenchmarkResolveCandidatePaths(b *testing.B) {
	dir := benchResolveDir()
	cands := realCandidates(b, dir, 200)
	if len(cands) == 0 {
		b.Skipf("no resolvable files under %s", dir)
	}
	volumeRoot := dir[:3] // "C:\"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := resolveCandidatePaths(volumeRoot, cands)
		if len(out) != len(cands) {
			b.Fatalf("resolved %d, want %d", len(out), len(cands))
		}
	}
	b.ReportMetric(float64(len(cands)), "files/op")
}

// BenchmarkTimestampQuery isolates the marginal cost the timestamp feature adds
// per displayed file: a single GetFileInformationByHandle on an already-open
// handle. This is the extra work resolveCandidatePaths does versus before.
func BenchmarkTimestampQuery(b *testing.B) {
	dir := benchResolveDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		b.Skipf("ReadDir(%s): %v", dir, err)
	}
	var h windows.Handle = windows.InvalidHandle
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		pw, err := windows.UTF16PtrFromString(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		hh, err := windows.CreateFile(pw,
			windows.FILE_READ_ATTRIBUTES,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING,
			windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err == nil {
			h = hh
			break
		}
	}
	if h == windows.InvalidHandle {
		b.Skipf("no openable file under %s", dir)
	}
	defer windows.CloseHandle(h)

	var info windows.ByHandleFileInformation
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := windows.GetFileInformationByHandle(h, &info); err != nil {
			b.Fatalf("GetFileInformationByHandle: %v", err)
		}
	}
}
