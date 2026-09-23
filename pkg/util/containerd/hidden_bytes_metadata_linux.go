// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/platforms"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	hiddenBytesMetadataBlobLimit  = 4 << 20
	hiddenBytesMetadataTotalLimit = 16 << 20
	hiddenBytesMetadataVisitLimit = 64
	hiddenBytesMetadataDepthLimit = 16
	hiddenBytesLayerLimit         = 4096
)

// HiddenBytesImage contains validated, ordered metadata shared by both scan routes.
type HiddenBytesImage struct {
	Manifest ocispec.Manifest
	DiffIDs  []digest.Digest
}

// ResolveHiddenBytesImage reads only local metadata, without unbounded image helpers.
// The expected config digest prevents a mutable tag from selecting a different image.
func ResolveHiddenBytesImage(ctx context.Context, provider content.Provider, target ocispec.Descriptor, platform platforms.MatchComparer, expectedConfig digest.Digest) (HiddenBytesImage, error) {
	if err := expectedConfig.Validate(); err != nil {
		return HiddenBytesImage{}, fmt.Errorf("invalid expected image config digest: %w", err)
	}
	r := hiddenBytesMetadataReader{provider: provider, ancestors: make(map[digest.Digest]bool)}
	result, found, err := r.resolve(ctx, target, platform, expectedConfig, 0)
	if ctx.Err() != nil {
		return HiddenBytesImage{}, ctx.Err()
	}
	if err != nil {
		return HiddenBytesImage{}, err
	}
	if !found {
		return HiddenBytesImage{}, fmt.Errorf("image reference no longer points to %s on the requested platform", expectedConfig)
	}
	return result, nil
}

type hiddenBytesMetadataReader struct {
	provider  content.Provider
	bytes     int64
	visits    int
	ancestors map[digest.Digest]bool
}

func (r *hiddenBytesMetadataReader) resolve(ctx context.Context, desc ocispec.Descriptor, platform platforms.MatchComparer, expected digest.Digest, depth int) (HiddenBytesImage, bool, error) {
	var empty HiddenBytesImage
	if err := ctx.Err(); err != nil {
		return empty, false, err
	}
	if depth >= hiddenBytesMetadataDepthLimit {
		return empty, false, errors.New("hidden byte metadata depth limit exceeded")
	}
	if r.ancestors[desc.Digest] {
		return empty, false, errors.New("hidden byte metadata descriptor cycle")
	}
	// Count even platform-filtered descriptors to bound index traversal.
	r.visits++
	if r.visits > hiddenBytesMetadataVisitLimit {
		return empty, false, errors.New("hidden byte metadata descriptor limit exceeded")
	}
	if platform != nil && desc.Platform != nil && !platform.Match(*desc.Platform) {
		return empty, false, nil
	}
	if !images.IsIndexType(desc.MediaType) && !images.IsManifestType(desc.MediaType) {
		return empty, false, fmt.Errorf("unsupported hidden byte metadata media type %q", desc.MediaType)
	}
	p, err := r.read(ctx, desc)
	if err != nil {
		return empty, false, err
	}
	r.ancestors[desc.Digest] = true
	defer delete(r.ancestors, desc.Digest)
	if images.IsIndexType(desc.MediaType) {
		var index ocispec.Index
		if err := json.Unmarshal(p, &index); err != nil {
			return empty, false, err
		}
		if index.SchemaVersion != 2 || (index.MediaType != "" && index.MediaType != desc.MediaType) {
			return empty, false, errors.New("invalid hidden byte image index")
		}
		// Prefer the same platform as containerd before reading compatible manifests
		// that may not have been pulled into the local content store.
		if platform != nil {
			sort.SliceStable(index.Manifests, func(i, j int) bool {
				if index.Manifests[i].Platform == nil {
					return false
				}
				if index.Manifests[j].Platform == nil {
					return true
				}
				return platform.Less(*index.Manifests[i].Platform, *index.Manifests[j].Platform)
			})
		}
		for _, child := range index.Manifests {
			result, found, err := r.resolve(ctx, child, platform, expected, depth+1)
			if err != nil || found {
				return result, found, err
			}
		}
		return empty, false, nil
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(p, &manifest); err != nil {
		return empty, false, err
	}
	if manifest.SchemaVersion != 2 || (manifest.MediaType != "" && manifest.MediaType != desc.MediaType) {
		return empty, false, errors.New("invalid hidden byte image manifest")
	}
	if manifest.Config.Digest != expected {
		return empty, false, nil
	}
	if !images.IsConfigType(manifest.Config.MediaType) {
		return empty, false, errors.New("unsupported hidden byte image config media type")
	}
	r.visits++
	if r.visits > hiddenBytesMetadataVisitLimit {
		return empty, false, errors.New("hidden byte metadata descriptor limit exceeded")
	}
	p, err = r.read(ctx, manifest.Config)
	if err != nil {
		return empty, false, err
	}
	var config ocispec.Image
	if err := json.Unmarshal(p, &config); err != nil {
		return empty, false, err
	}
	if platform != nil && !platform.Match(config.Platform) {
		return empty, false, nil
	}
	if config.OS != "linux" {
		return empty, false, errors.New("hidden bytes requires Linux image layers")
	}
	if config.RootFS.Type != "layers" || len(manifest.Layers) == 0 || len(manifest.Layers) > hiddenBytesLayerLimit || len(manifest.Layers) != len(config.RootFS.DiffIDs) {
		return empty, false, errors.New("invalid hidden byte image layer count or rootfs type")
	}
	for i, layer := range manifest.Layers {
		if err := ctx.Err(); err != nil {
			return empty, false, err
		}
		if err := layer.Digest.Validate(); err != nil {
			return empty, false, fmt.Errorf("invalid layer digest: %w", err)
		}
		if err := config.RootFS.DiffIDs[i].Validate(); err != nil {
			return empty, false, fmt.Errorf("invalid layer diffID: %w", err)
		}
		if layer.Size < 0 {
			return empty, false, errors.New("invalid negative layer size")
		}
		if _, err := hiddenBytesLayerCompression(layer.MediaType); err != nil {
			return empty, false, err
		}
	}
	return HiddenBytesImage{Manifest: manifest, DiffIDs: config.RootFS.DiffIDs}, true, nil
}

func (r *hiddenBytesMetadataReader) read(ctx context.Context, desc ocispec.Descriptor) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := desc.Digest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid metadata digest: %w", err)
	}
	if desc.Size < 0 || desc.Size > hiddenBytesMetadataBlobLimit || len(desc.Data) > hiddenBytesMetadataBlobLimit {
		return nil, errors.New("hidden byte metadata blob size limit exceeded")
	}
	if desc.Size > hiddenBytesMetadataTotalLimit-r.bytes {
		return nil, errors.New("hidden byte metadata total size limit exceeded")
	}
	r.bytes += desc.Size
	var p []byte
	if desc.Data != nil {
		if int64(len(desc.Data)) != desc.Size {
			return nil, errors.New("inline metadata size does not match descriptor")
		}
		p = desc.Data
	} else {
		reader, err := r.provider.ReaderAt(ctx, desc)
		if err != nil {
			return nil, fmt.Errorf("read local image metadata: %w", err)
		}
		defer reader.Close()
		if reader.Size() != desc.Size {
			return nil, errors.New("local metadata size does not match descriptor")
		}
		p = make([]byte, int(desc.Size))
		for offset := 0; offset < len(p); {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			end := min(offset+32*1024, len(p))
			n, err := reader.ReadAt(p[offset:end], int64(offset))
			if err != nil && !(err == io.EOF && n == end-offset) {
				return nil, err
			}
			if n != end-offset {
				return nil, io.ErrUnexpectedEOF
			}
			offset = end
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	verifier := desc.Digest.Verifier()
	_, _ = verifier.Write(p)
	if !verifier.Verified() {
		return nil, errors.New("image metadata digest mismatch")
	}
	return p, nil
}

func hiddenBytesLayerCompression(mediaType string) (string, error) {
	switch mediaType {
	case ocispec.MediaTypeImageLayer, images.MediaTypeDockerSchema2Layer, images.MediaTypeDockerSchema2LayerForeign,
		"application/vnd.oci.image.layer.nondistributable.v1.tar":
		return "", nil
	case ocispec.MediaTypeImageLayerGzip, images.MediaTypeDockerSchema2LayerGzip, images.MediaTypeDockerSchema2LayerForeignGzip,
		"application/vnd.oci.image.layer.nondistributable.v1.tar+gzip":
		return "gzip", nil
	case ocispec.MediaTypeImageLayerZstd, images.MediaTypeDockerSchema2LayerZstd,
		"application/vnd.oci.image.layer.nondistributable.v1.tar+zstd":
		return "zstd", nil
	default:
		return "", fmt.Errorf("unsupported hidden byte layer media type %q", mediaType)
	}
}
