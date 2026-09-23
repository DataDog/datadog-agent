// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

type imageLayerSnapshotter map[string]snapshots.Info

func (s imageLayerSnapshotter) Stat(_ context.Context, key string) (snapshots.Info, error) {
	if info, ok := s[key]; ok {
		return info, nil
	}
	return snapshots.Info{}, errdefs.ErrNotFound
}

func TestBuildImageLayers(t *testing.T) {
	diffIDs := []digest.Digest{digest.FromString("base"), digest.FromString("top")}
	chain := ComputeChainIDs(diffIDs)
	snap := imageLayerSnapshotter{chain[0].String(): {}, chain[1].String(): {Parent: chain[0].String()}}
	manifest := ocispec.Manifest{Layers: []ocispec.Descriptor{{Digest: digest.FromString("blob-base")}, {Digest: digest.FromString("blob-top")}}}
	mounts := []mount.Mount{{Type: "overlay", Options: []string{"lowerdir=/top:/base", "index=off"}}}
	got, err := BuildImageLayers(t.Context(), snap, diffIDs, manifest, mounts)
	require.NoError(t, err)
	require.Equal(t, []ImageLayer{{DiffID: diffIDs[0].String(), Digest: manifest.Layers[0].Digest.String(), Path: "/base"}, {DiffID: diffIDs[1].String(), Digest: manifest.Layers[1].Digest.String(), Path: "/top"}}, got)
	mounts[0].Options = append(mounts[0].Options, "userxattr")
	got, err = BuildImageLayers(t.Context(), snap, diffIDs, manifest, mounts)
	require.NoError(t, err)
	for _, layer := range got {
		require.True(t, layer.UserXAttr)
	}
	snap[chain[1].String()] = snapshots.Info{Parent: "wrong"}
	got, err = BuildImageLayers(t.Context(), snap, diffIDs, manifest, mounts)
	require.ErrorIs(t, err, ErrLayerChainMismatch)
	require.Nil(t, got)
}

func TestImageViewCleanupIsOwnedAndCancellationIndependent(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var removals, releases int
	check := func(ctx context.Context) {
		require.NoError(t, ctx.Err())
		namespace, ok := namespaces.Namespace(ctx)
		require.True(t, ok)
		require.Equal(t, "k8s.io", namespace)
		_, hasDeadline := ctx.Deadline()
		require.True(t, hasDeadline)
	}
	cleanup := imageViewCleanup("k8s.io", func(ctx context.Context) error { check(ctx); removals++; return nil }, func(ctx context.Context) error { check(ctx); releases++; return nil })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); require.NoError(t, cleanup(ctx)) }()
	}
	wg.Wait()
	require.Equal(t, 1, removals)
	require.Equal(t, 1, releases)
	// A second operation's cleanup has independent ownership.
	other := imageViewCleanup("k8s.io", func(context.Context) error { removals++; return nil }, func(context.Context) error { releases++; return nil })
	require.NoError(t, other(ctx))
	require.Equal(t, 2, removals)
	require.Equal(t, 2, releases)
}

func TestImageViewCleanupReleasesLeaseAfterViewFailure(t *testing.T) {
	want := errors.New("view removal failure")
	released := false
	cleanup := imageViewCleanup("k8s.io", func(context.Context) error { return want }, func(context.Context) error { released = true; return errdefs.ErrNotFound })
	require.ErrorIs(t, cleanup(t.Context()), want)
	require.True(t, released)
	cleanup = imageViewCleanup("k8s.io", func(context.Context) error { return errdefs.ErrNotFound }, func(context.Context) error { return errdefs.ErrNotFound })
	require.NoError(t, cleanup(t.Context()))
}

func TestImageViewCleanupUsesSeparateReleaseContexts(t *testing.T) {
	var viewCtx context.Context
	cleanup := imageViewCleanup("k8s.io", func(ctx context.Context) error {
		viewCtx = ctx
		return context.DeadlineExceeded
	}, func(ctx context.Context) error {
		// The view's budget is cancelled before the fresh lease budget starts.
		require.ErrorIs(t, viewCtx.Err(), context.Canceled)
		require.NoError(t, ctx.Err())
		require.NotSame(t, viewCtx, ctx)
		return nil
	})
	require.ErrorIs(t, cleanup(t.Context()), context.DeadlineExceeded)
}
