// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestComputeHtlHash(t *testing.T) {
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
					hash, err := computeHtlHash(f)
					if err != nil {
						t.Fatalf("failed to compute htl hash: %v", err)
					}
					hashes[hash] = struct{}{}
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
			hash, err := computeHtlHash(reader)
			if err != nil {
				t.Fatalf("failed to compute htl hash: %v", err)
			}
			hashes[hash] = struct{}{}
		}
		require.Equal(t, maxSize, len(hashes))
	})
}
