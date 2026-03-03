// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListPids(t *testing.T) {

	t.Run("pages all entries", func(t *testing.T) {
		const pageSize = 2
		dir := t.TempDir()
		for _, name := range []string{"101", "202"} {
			require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o755))
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "not-a-pid"), []byte(""), 0o644))
		for i := range pageSize * 8 {
			p := fmt.Sprintf("not-a-pid-%d", i)
			require.NoError(t, os.Mkdir(filepath.Join(dir, p), 0o755))
		}
		require.NoError(t, os.Mkdir(filepath.Join(dir, "303"), 0o755))
		require.NoError(t, os.Mkdir(filepath.Join(dir, "0"), 0o755)) //ignored

		seq := listPidsChunks(dir, pageSize)
		pages, err := collectErr(seq)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(pages), 2)

		// Make sure no page is larger than the page size.
		for _, page := range pages {
			require.LessOrEqual(t, len(page), pageSize)
		}

		flat := slices.Sorted(flatten(slices.Values(pages)))
		require.Equal(t, []uint32{101, 202, 303}, flat)
	})
	t.Run("directory error", func(t *testing.T) {
		seq := listPidsChunks(filepath.Join(t.TempDir(), "missing"), 4)
		_, err := collectErr(seq)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
	t.Run("no empty pages", func(t *testing.T) {
		dir := t.TempDir()
		const pageSize = 2
		for i := range pageSize * 8 {
			p := fmt.Sprintf("not-a-pid-%d", i)
			require.NoError(t, os.Mkdir(filepath.Join(dir, p), 0o755))
		}
		seq := listPidsChunks(dir, pageSize)
		pageCount := 0
		seq(func(_ []uint32, pageErr error) bool {
			pageCount++
			require.NoError(t, pageErr)
			// Stop after the first page to ensure early termination is
			// honored.
			return false
		})
		require.Equal(t, 0, pageCount)
	})
	t.Run("caller stops early", func(t *testing.T) {
		dir := t.TempDir()
		const pageSize = 3
		for i := range pageSize * 5 {
			p := strconv.Itoa(i + 1)
			require.NoError(t, os.Mkdir(filepath.Join(dir, p), 0o755))
		}
		seq := listPidsChunks(dir, pageSize)
		pageCount := 0
		seq(func(_ []uint32, pageErr error) bool {
			pageCount++
			require.NoError(t, pageErr)
			// Stop after the first page to ensure early termination is
			// honored.
			return false
		})
		require.Equal(t, 1, pageCount)
	})
}

func flatten[T any](seq iter.Seq[[]T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for page := range seq {
			for _, v := range page {
				if !yield(v) {
					return
				}
			}
		}
	}
}

func collectErr[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var ret []T
	for v, err := range seq {
		if err != nil {
			return nil, err
		}
		ret = append(ret, v)
	}
	return ret, nil
}
