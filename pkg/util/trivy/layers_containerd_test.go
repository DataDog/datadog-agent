// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && containerd

package trivy

import (
	"context"
	"strings"
	"testing"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// canonicalEmptyDiffID is sha256 of an empty tar. Trivy used to
// special-case this value because the old positional pairing kept
// reusing it across images sharing the same canonical empty layer.
const canonicalEmptyDiffID = "sha256:5f70bf18a086007016e948b04aed3b82103a36bea41755b6cddfaf10ace3c6ef"

// d wraps digest.FromString for terser test fixtures.
func d(s string) digest.Digest { return digest.FromString(s) }

func TestComputeChainIDs(t *testing.T) {
	// computeChainIDs must copy before delegating to identity.ChainIDs
	// (which mutates in place) and return the whole chain bottom-up, with
	// the base chainID equal to the base diff_id.
	diffIDs := []digest.Digest{d("base"), d("mid"), d("top")}
	orig := append([]digest.Digest{}, diffIDs...)

	got := computeChainIDs(diffIDs)

	require.Len(t, got, len(diffIDs))
	assert.Equal(t, diffIDs[0], got[0])
	for i := range orig {
		require.Equal(t, orig[i], diffIDs[i])
	}

	assert.Nil(t, computeChainIDs(nil))
}

// fakeSnapshotter is a minimal snapshotterStat. An unset entry returns
// ErrNotFound, mirroring containerd's behaviour for unknown chainIDs.
type fakeSnapshotter struct {
	infos map[string]snapshots.Info
}

func (f *fakeSnapshotter) Stat(_ context.Context, key string) (snapshots.Info, error) {
	if info, ok := f.infos[key]; ok {
		return info, nil
	}
	return snapshots.Info{}, errdefs.ErrNotFound
}

// snapshotterFromChain populates a fakeSnapshotter so Stat(chain[i])
// returns Parent=chain[i-1] (and empty Parent for the base).
func snapshotterFromChain(chain []digest.Digest) *fakeSnapshotter {
	infos := make(map[string]snapshots.Info, len(chain))
	for i, c := range chain {
		info := snapshots.Info{Name: c.String()}
		if i > 0 {
			info.Parent = chain[i-1].String()
		}
		infos[c.String()] = info
	}
	return &fakeSnapshotter{infos: infos}
}

func TestVerifyChainAgainstSnapshotter(t *testing.T) {
	chain := []digest.Digest{d("c0"), d("c0c1"), d("c0c1c2")}

	t.Run("happy_path", func(t *testing.T) {
		require.NoError(t, verifyChainAgainstSnapshotter(t.Context(), snapshotterFromChain(chain), chain))
	})

	t.Run("missing_snapshot", func(t *testing.T) {
		f := snapshotterFromChain(chain)
		delete(f.infos, chain[1].String())
		err := verifyChainAgainstSnapshotter(t.Context(), f, chain)
		require.ErrorIs(t, err, errdefs.ErrNotFound)
	})

	t.Run("wrong_parent", func(t *testing.T) {
		f := snapshotterFromChain(chain)
		f.infos[chain[1].String()] = snapshots.Info{
			Name:   chain[1].String(),
			Parent: d("not-the-parent").String(),
		}
		err := verifyChainAgainstSnapshotter(t.Context(), f, chain)
		require.ErrorIs(t, err, errLayerChainMismatch)
	})

	t.Run("base_has_nonempty_parent", func(t *testing.T) {
		f := snapshotterFromChain(chain)
		f.infos[chain[0].String()] = snapshots.Info{
			Name:   chain[0].String(),
			Parent: "uh-oh",
		}
		err := verifyChainAgainstSnapshotter(t.Context(), f, chain)
		require.ErrorIs(t, err, errLayerChainMismatch)
	})
}

// manifestForDiffIDs builds an OCI manifest with one Descriptor per
// diff_id. Layer Digests in the manifest are derived from a separate
// namespace so a positional confusion of Digest with DiffID is
// detectable in the assertions below.
func manifestForDiffIDs(diffIDs []digest.Digest) ocispec.Manifest {
	layers := make([]ocispec.Descriptor, len(diffIDs))
	for i := range diffIDs {
		layers[i] = ocispec.Descriptor{
			Digest:    d("blob-" + diffIDs[i].String()),
			MediaType: ocispec.MediaTypeImageLayerGzip,
			Size:      int64(1024 * (i + 1)),
		}
	}
	return ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Layers:    layers,
	}
}

// mountsFromTopDownPaths wraps paths in a single overlay mount option.
func mountsFromTopDownPaths(paths []string) []mount.Mount {
	return []mount.Mount{{
		Type:    "overlay",
		Source:  "overlay",
		Options: []string{"lowerdir=" + strings.Join(paths, ":")},
	}}
}

func TestBuildContainerdLayerPaths(t *testing.T) {
	const imgName = "test-image"

	// Five-layer image with the canonical empty-layer DiffID in the
	// middle, exercising a historical failure mode.
	diffIDs := []digest.Digest{
		d("base"),
		d("apt-deps"),
		digest.Digest(canonicalEmptyDiffID),
		d("rails"),
		d("entrypoint"),
	}
	manifest := manifestForDiffIDs(diffIDs)
	topDownPaths := []string{
		"/snap/path-entrypoint",
		"/snap/path-rails",
		"/snap/path-canonical-empty",
		"/snap/path-apt-deps",
		"/snap/path-base",
	}
	chain := computeChainIDs(diffIDs)
	snap := snapshotterFromChain(chain)

	t.Run("happy_path", func(t *testing.T) {
		got, err := buildContainerdLayerPaths(t.Context(), snap, imgName, diffIDs, manifest, mountsFromTopDownPaths(topDownPaths))
		require.NoError(t, err)
		require.Len(t, got, len(diffIDs))
		for i, want := range []struct {
			diffID digest.Digest
			path   string
		}{
			{diffIDs[0], "/snap/path-base"},
			{diffIDs[1], "/snap/path-apt-deps"},
			{diffIDs[2], "/snap/path-canonical-empty"},
			{diffIDs[3], "/snap/path-rails"},
			{diffIDs[4], "/snap/path-entrypoint"},
		} {
			assert.Equal(t, want.diffID.String(), got[i].DiffID)
			assert.Equal(t, want.path, got[i].Path)
			assert.Equal(t, manifest.Layers[i].Digest.String(), got[i].Digest)
			assert.NotEqualf(t, got[i].DiffID, got[i].Digest, "[%d] manifest digest must differ from diff_id", i)
		}
	})

	t.Run("count_mismatch_paths_too_few", func(t *testing.T) {
		_, err := buildContainerdLayerPaths(t.Context(), snap, imgName, diffIDs, manifest, mountsFromTopDownPaths(topDownPaths[:3]))
		require.ErrorIs(t, err, errLayerCountMismatch)
	})

	t.Run("count_mismatch_paths_too_many", func(t *testing.T) {
		extra := append([]string{}, topDownPaths...)
		extra = append(extra, "/snap/path-spurious-upper")
		_, err := buildContainerdLayerPaths(t.Context(), snap, imgName, diffIDs, manifest, mountsFromTopDownPaths(extra))
		require.ErrorIs(t, err, errLayerCountMismatch)
	})

	t.Run("empty_diff_ids", func(t *testing.T) {
		_, err := buildContainerdLayerPaths(t.Context(), snap, imgName, nil, manifest, nil)
		require.Error(t, err)
	})

	t.Run("manifest_short_keeps_diffid_path_drops_digest", func(t *testing.T) {
		shortManifest := manifestForDiffIDs(diffIDs[:3])
		got, err := buildContainerdLayerPaths(t.Context(), snap, imgName, diffIDs, shortManifest, mountsFromTopDownPaths(topDownPaths))
		require.NoError(t, err)
		for i := range got {
			assert.NotEmptyf(t, got[i].DiffID, "[%d] DiffID is empty", i)
			assert.NotEmptyf(t, got[i].Path, "[%d] Path is empty", i)
			assert.Emptyf(t, got[i].Digest, "[%d] Digest = %q, want empty when manifest is short", i, got[i].Digest)
		}
	})

	t.Run("snapshotter_chain_mismatch", func(t *testing.T) {
		badSnap := snapshotterFromChain(chain)
		badSnap.infos[chain[2].String()] = snapshots.Info{
			Name:   chain[2].String(),
			Parent: d("not-the-parent").String(),
		}
		_, err := buildContainerdLayerPaths(t.Context(), badSnap, imgName, diffIDs, manifest, mountsFromTopDownPaths(topDownPaths))
		require.ErrorIs(t, err, errLayerChainMismatch)
	})

	t.Run("never_emits_wrong_pair", func(t *testing.T) {
		// Invariant: no LayerPath.Digest should ever equal any DiffID.
		// The desync we guard against took exactly that shape.
		got, err := buildContainerdLayerPaths(t.Context(), snap, imgName, diffIDs, manifest, mountsFromTopDownPaths(topDownPaths))
		require.NoError(t, err)
		diffIDSet := make(map[string]struct{}, len(diffIDs))
		for _, x := range diffIDs {
			diffIDSet[x.String()] = struct{}{}
		}
		for i, lp := range got {
			assert.NotContainsf(t, diffIDSet, lp.Digest, "[%d] LayerPath.Digest = %q is also a DiffID; pairing desync", i, lp.Digest)
		}
	})
}
