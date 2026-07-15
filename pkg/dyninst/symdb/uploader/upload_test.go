// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
)

// fakeEncoder is a PackageEncoder that records each Add/Flush call.
type fakeEncoder struct {
	added       []symdb.Package
	addedAgent  []string
	flushedAt   []bool // one entry per Flush, true if final
	currentSize int
	growPerAdd  int
	flushErr    error
	addErr      error
}

func (f *fakeEncoder) AddPackage(pkg symdb.Package, agentVersion string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, pkg)
	f.addedAgent = append(f.addedAgent, agentVersion)
	f.currentSize += f.growPerAdd
	return nil
}

func (f *fakeEncoder) Size() int { return f.currentSize }

func (f *fakeEncoder) Flush(_ context.Context, final bool) error {
	if f.flushErr != nil {
		return f.flushErr
	}
	f.flushedAt = append(f.flushedAt, final)
	f.currentSize = 0
	return nil
}

// pkgIter builds a Seq2 yielding the given packages, with Final set on the
// last one. If errAt is non-zero, it yields an error on the (1-indexed) i-th
// yield instead of a package.
func pkgIter(pkgs []symdb.Package, errAt int) iter.Seq2[symdb.PackageWithFinal, error] {
	return func(yield func(symdb.PackageWithFinal, error) bool) {
		for i, p := range pkgs {
			if errAt == i+1 {
				yield(symdb.PackageWithFinal{}, errors.New("synthetic iter error"))
				return
			}
			final := i == len(pkgs)-1
			if !yield(symdb.PackageWithFinal{Package: p, Final: final}, nil) {
				return
			}
		}
	}
}

func TestRunUploadLoop(t *testing.T) {
	t.Run("flushes_on_final", func(t *testing.T) {
		enc := &fakeEncoder{}
		pkgs := []symdb.Package{
			{Name: "main"},
			{Name: "github.com/example/foo"},
		}
		stats, err := RunUploadLoop(
			context.Background(), enc, pkgIter(pkgs, 0),
			"v1.2.3", 1<<30,
		)
		require.NoError(t, err)
		assert.Equal(t, 2, stats.Packages)
		assert.Equal(t, 0, stats.Functions)
		// One Flush, on the final package.
		assert.Equal(t, []bool{true}, enc.flushedAt)
		assert.Equal(t, 1, stats.Batches)
		assert.Equal(t, []string{"v1.2.3", "v1.2.3"}, enc.addedAgent)
	})

	t.Run("flushes_when_size_threshold_reached", func(t *testing.T) {
		enc := &fakeEncoder{growPerAdd: 100}
		pkgs := []symdb.Package{
			{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"},
		}
		stats, err := RunUploadLoop(
			context.Background(), enc, pkgIter(pkgs, 0),
			"v1", 250,
		)
		require.NoError(t, err)
		assert.Equal(t, 4, stats.Packages)
		// After "c" Size=300 > 250 → flush(false). After "d" final flush(true).
		assert.Equal(t, []bool{false, true}, enc.flushedAt)
		assert.Equal(t, 2, stats.Batches)
	})

	t.Run("propagates_iterator_error", func(t *testing.T) {
		enc := &fakeEncoder{}
		_, err := RunUploadLoop(
			context.Background(), enc, pkgIter([]symdb.Package{{Name: "a"}}, 1),
			"v", 1<<30,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "synthetic iter error")
	})

	t.Run("propagates_add_error", func(t *testing.T) {
		enc := &fakeEncoder{addErr: errors.New("encode boom")}
		_, err := RunUploadLoop(
			context.Background(), enc, pkgIter([]symdb.Package{{Name: "a"}}, 0),
			"v", 1<<30,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "encode boom")
	})

	t.Run("propagates_flush_error", func(t *testing.T) {
		enc := &fakeEncoder{flushErr: ErrUpload}
		_, err := RunUploadLoop(
			context.Background(), enc, pkgIter([]symdb.Package{{Name: "a"}}, 0),
			"v", 1<<30,
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUpload)
	})

	t.Run("respects_canceled_context", func(t *testing.T) {
		enc := &fakeEncoder{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := RunUploadLoop(
			ctx, enc, pkgIter([]symdb.Package{{Name: "a"}}, 0),
			"v", 1<<30,
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
