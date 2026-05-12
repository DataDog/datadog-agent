// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
)

// benchBinaryEnv names the environment variable that points at the
// binary used by BenchmarkExtractAndUpload. The benchmark is skipped
// when the variable is unset or points at a missing file so that the
// default test invocation (no env, no fixture binary in the repo)
// remains a no-op.
const benchBinaryEnv = "SYMDBCLI_BENCH_BINARY"

// BenchmarkExtractAndUpload exercises the full symdbcli upload pipeline
// against a binary supplied via the SYMDBCLI_BENCH_BINARY environment
// variable. It runs an in-process httptest.Server whose handler streams
// each request body into io.Discard, so the benchmark measures
// extraction + JSON + gzip + multipart + HTTP send (everything the real
// path does) without buffering the upload payload in heap.
//
// Run with:
//
//	SYMDBCLI_BENCH_BINARY=/path/to/binary \
//	  go test -tags linux_bpf,test -run='^$' -bench=BenchmarkExtractAndUpload \
//	  ./pkg/dyninst/symdb/cli/...
//
// The benchmark uses an on-disk object cache rooted at b.TempDir() to
// match the system-probe DiskCacheEnabled configuration. The temp dir
// is automatically cleaned up by the testing package on completion.
func BenchmarkExtractAndUpload(b *testing.B) {
	binaryPath := os.Getenv(benchBinaryEnv)
	if binaryPath == "" {
		b.Skipf("%s not set; skipping", benchBinaryEnv)
	}
	if _, err := os.Stat(binaryPath); err != nil {
		b.Skipf("%s=%q: %v; skipping", benchBinaryEnv, binaryPath, err)
	}

	diskCache, err := object.NewDiskCache(object.DiskCacheConfig{
		DirPath:                b.TempDir(),
		MaxTotalBytes:          2 << 30, // 2 GiB, matches symdbcli/system-probe default
		RequiredDiskSpaceBytes: 512 << 20,
	})
	if err != nil {
		b.Fatalf("NewDiskCache: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stream the body to io.Discard so we exercise the network read
		// path without holding the upload in heap.
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	extractOpts := symdb.ExtractOptions{
		Scope:     symdb.ExtractScopeModulesFromSameOrg,
		DiskCache: diskCache,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		it, err := symdb.PackagesIterator(binaryPath, diskCache, extractOpts)
		if err != nil {
			b.Fatalf("PackagesIterator: %v", err)
		}
		enc, err := uploader.NewBatchEncoder(
			srv.URL,
			"bench-service", "bench-version", "bench-runtime-id",
			uuid.New(), diskCache, nil,
		)
		if err != nil {
			b.Fatalf("NewBatchEncoder: %v", err)
		}
		if _, err := uploader.RunUploadLoop(
			ctx, enc, it, "bench-version",
			uploader.DefaultFlushThresholdBytes,
		); err != nil {
			_ = enc.Close()
			b.Fatalf("RunUploadLoop: %v", err)
		}
		if err := enc.Close(); err != nil {
			b.Fatalf("BatchEncoder.Close: %v", err)
		}
	}
}
