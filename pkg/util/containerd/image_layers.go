// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ImageLayer identifies an unpacked image layer, ordered from base to top.
type ImageLayer struct {
	DiffID string
	Digest string
	Path   string
	// UserXAttr selects the user.overlay namespace used by an overlay userxattr mount.
	// Without it, only trusted.overlay metadata controls visibility.
	UserXAttr bool
}

// ErrLayerChainMismatch means snapshot ancestry disagrees with image metadata.
var ErrLayerChainMismatch = errors.New("snapshotter chain does not match image config")

// ErrLayerCountMismatch means the view cannot be paired to the image layers.
var ErrLayerCountMismatch = errors.New("snapshot layer count does not match image config")

// SnapshotterStat is the snapshot metadata needed to validate image layer order.
type SnapshotterStat interface {
	Stat(context.Context, string) (snapshots.Info, error)
}

// ComputeChainIDs returns every cumulative chain ID without modifying diffIDs.
func ComputeChainIDs(diffIDs []digest.Digest) []digest.Digest {
	out := slices.Clone(diffIDs)
	identity.ChainIDs(out)
	return out
}

// VerifyImageLayerChain verifies the parent of every image snapshot.
func VerifyImageLayerChain(ctx context.Context, s SnapshotterStat, chainIDs []digest.Digest) error {
	for i := len(chainIDs) - 1; i >= 0; i-- {
		info, err := s.Stat(ctx, chainIDs[i].String())
		if err != nil {
			return fmt.Errorf("snapshotter stat for %s: %w", chainIDs[i], err)
		}
		var parent string
		if i > 0 {
			parent = chainIDs[i-1].String()
		}
		if info.Parent != parent {
			return fmt.Errorf("%w: chainID %s has parent %q, expected %q", ErrLayerChainMismatch, chainIDs[i], info.Parent, parent)
		}
	}
	return nil
}

// ExtractImageLayerPaths returns mount paths in overlayfs top-down order.
// This also preserves the existing SBOM handling of snapshotter mount options.
func ExtractImageLayerPaths(mounts []mount.Mount) []string {
	var paths []string
	for _, m := range mounts {
		found := false
		for _, opt := range m.Options {
			for _, prefix := range []string{"upperdir=", "lowerdir="} {
				if value, ok := strings.CutPrefix(opt, prefix); ok {
					paths = append(paths, strings.Split(value, ":")...)
					found = true
				}
			}
		}
		if !found && m.Type == "bind" && m.Source != "" {
			paths = append(paths, m.Source)
		}
	}
	return paths
}

// BuildImageLayers pairs validated snapshot paths with ordered image metadata.
// A manifest with a different layer count has no safe positional digest mapping.
func BuildImageLayers(ctx context.Context, s SnapshotterStat, diffIDs []digest.Digest, manifest ocispec.Manifest, mounts []mount.Mount) ([]ImageLayer, error) {
	if len(diffIDs) == 0 {
		return nil, errors.New("image has no diff_ids")
	}
	if err := VerifyImageLayerChain(ctx, s, ComputeChainIDs(diffIDs)); err != nil {
		return nil, err
	}
	paths := ExtractImageLayerPaths(mounts)
	if len(paths) != len(diffIDs) {
		return nil, fmt.Errorf("%w: %d paths vs %d diff_ids", ErrLayerCountMismatch, len(paths), len(diffIDs))
	}
	layers := make([]ImageLayer, len(diffIDs))
	userXAttr := len(mounts) == 1 && slices.Contains(mounts[0].Options, "userxattr")
	for i := range diffIDs {
		layers[i] = ImageLayer{DiffID: diffIDs[i].String(), Path: paths[len(paths)-1-i], UserXAttr: userXAttr}
		if len(manifest.Layers) == len(diffIDs) {
			layers[i].Digest = manifest.Layers[i].Digest.String()
		}
	}
	return layers, nil
}

// AcquireImageLayers holds a private lease and read-only view of native overlayfs
// snapshots. The caller must call cleanup even if subsequent collection fails.
func AcquireImageLayers(ctx context.Context, client ContainerdItf, namespace string, manifest ocispec.Manifest, diffIDs []digest.Digest, expiration time.Duration) ([]ImageLayer, func(context.Context) error, error) {
	ctx = namespaces.WithNamespace(ctx, namespace)
	if len(diffIDs) == 0 {
		return nil, nil, errors.New("image has no diff_ids")
	}
	mounts, cleanup, err := acquireImageMountsForChain(ctx, client.RawClient(), expiration, namespace, identity.ChainID(diffIDs).String(), "overlayfs")
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) ([]ImageLayer, func(context.Context) error, error) {
		return nil, nil, errors.Join(err, cleanup(ctx))
	}
	if err := validateHiddenBytesMounts(mounts, len(diffIDs)); err != nil {
		return fail(err)
	}
	layers, err := BuildImageLayers(ctx, client.RawClient().SnapshotService("overlayfs"), diffIDs, manifest, mounts)
	if err != nil {
		return fail(err)
	}
	return layers, cleanup, nil
}

func validateHiddenBytesMounts(mounts []mount.Mount, count int) error {
	if len(mounts) != 1 {
		return errors.New("hidden bytes requires one native overlayfs mount")
	}
	m := mounts[0]
	readonly, lower := false, 0
	for _, opt := range m.Options {
		switch {
		case opt == "ro":
			readonly = true
		case opt == "rbind", opt == "bind":
		case strings.HasPrefix(opt, "lowerdir=") && m.Type == "overlay":
			lower++
		case opt == "userxattr", opt == "index=off":
		default:
			return fmt.Errorf("unsupported image snapshot mount option %q", opt)
		}
	}
	// Native overlayfs views have only lowerdir and need not include an explicit ro.
	if (m.Type != "overlay" && !(m.Type == "bind" && count == 1 && readonly)) || (m.Type == "overlay" && lower != 1) {
		return errors.New("hidden bytes requires a read-only overlayfs view or a single-layer bind")
	}
	for _, path := range ExtractImageLayerPaths(mounts) {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\\,:\x00") {
			return fmt.Errorf("unsupported image layer path %q", path)
		}
	}
	return nil
}
