// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package main

import (
	"context"
	"os"
	"testing"

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

// BenchmarkExtractAndUpload exercises the full symdbcli upload loop —
// DWARF iteration, package-to-scope conversion, JSON encoding, and
// gzip compression — against a binary supplied via the
// SYMDBCLI_BENCH_BINARY environment variable. The output is dropped
// by noopSink, so the benchmark measures everything the real upload
// path does except the HTTP request itself.
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

	ctx := context.Background()
	extractOpts := symdb.ExtractOptions{
		Scope:     symdb.ExtractScopeModulesFromSameOrg,
		DiskCache: diskCache,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		var stats symbolStats
		if err := extractAndUpload(
			ctx,
			binaryPath,
			diskCache,
			extractOpts,
			noopSink{},
			"bench-version",
			uploader.DefaultFlushThresholdBytes,
			true, // silent
			&stats,
		); err != nil {
			b.Fatalf("extractAndUpload: %v", err)
		}
	}
}
