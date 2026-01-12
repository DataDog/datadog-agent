// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package htlhash_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/htlhash"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestCompute(t *testing.T) {
	// Just test that it doesn't fail and comes up with unique strings for every
	// test program.
	t.Run("programs are unique", func(t *testing.T) {
		hashes := make(map[string]struct{})
		programs := testprogs.MustGetPrograms(t)
		cfgs := testprogs.MustGetCommonConfigs(t)
		expectedNumHashes := 0
		for _, prog := range programs {
			for _, cfg := range cfgs {
				expectedNumHashes++
				t.Run(fmt.Sprintf(
					"%s-%s-%s", prog, cfg.GOARCH, cfg.GOTOOLCHAIN,
				), func(t *testing.T) {
					bin := testprogs.MustGetBinary(t, prog, cfg)
					f, err := os.Open(bin)
					if err != nil {
						t.Fatalf("failed to open binary: %v", err)
					}
					defer f.Close()
					hash, err := htlhash.Compute(f)
					if err != nil {
						t.Fatalf("failed to compute htl hash: %v", err)
					}
					hashes[hash.String()] = struct{}{}
				})
			}
		}
		require.Equal(t, expectedNumHashes, len(hashes))
	})
	t.Run("small files are unique", func(t *testing.T) {
		const maxSize = 4096 * 3
		buf := make([]byte, maxSize)
		hashes := make(map[string]struct{}, maxSize)
		reader := bytes.NewReader(nil)
		for i := 0; i < maxSize; i++ {
			reader.Reset(buf[:i])
			hash, err := htlhash.Compute(reader)
			if err != nil {
				t.Fatalf("failed to compute htl hash: %v", err)
			}
			hashes[hash.String()] = struct{}{}
		}
		require.Equal(t, maxSize, len(hashes))
	})
}

// BenchmarkCompute measures the performance of computing the HTL hash
// for one of the real test binaries. The benchmark opens the binary once and
// re-uses the file handle for every iteration, seeking back to the beginning
// before each hash computation.
func BenchmarkCompute(b *testing.B) {
	cfgs := testprogs.MustGetCommonConfigs(b)
	binPath := testprogs.MustGetBinary(b, "simple", cfgs[0])

	f, err := os.Open(binPath)
	require.NoError(b, err)
	defer f.Close()

	b.ResetTimer()
	for b.Loop() {
		_, err := f.Seek(0, io.SeekStart)
		require.NoError(b, err)

		_, err = htlhash.Compute(f)
		require.NoError(b, err)
	}
}
