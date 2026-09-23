// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/platforms"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

type hiddenMetadataProvider struct {
	blobs  map[digest.Digest][]byte
	opened int
	closed int
	onRead func()
}

type hiddenMetadataReaderAt struct {
	*bytes.Reader
	provider *hiddenMetadataProvider
}

func (r *hiddenMetadataReaderAt) Close() error {
	r.provider.closed++
	return nil
}

func (r *hiddenMetadataReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if r.provider.onRead != nil {
		r.provider.onRead()
	}
	return r.Reader.ReadAt(p, offset)
}

func (p *hiddenMetadataProvider) ReaderAt(_ context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	data, ok := p.blobs[desc.Digest]
	if !ok {
		return nil, errors.New("missing local blob")
	}
	p.opened++
	return &hiddenMetadataReaderAt{Reader: bytes.NewReader(data), provider: p}, nil
}

func (p *hiddenMetadataProvider) add(t *testing.T, value any, mediaType string) ocispec.Descriptor {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	desc := ocispec.Descriptor{MediaType: mediaType, Digest: digest.FromBytes(data), Size: int64(len(data))}
	p.blobs[desc.Digest] = data
	return desc
}

func hiddenMetadataFixture(t *testing.T, mutate func(*ocispec.Image, *ocispec.Manifest)) (*hiddenMetadataProvider, ocispec.Descriptor, digest.Digest) {
	t.Helper()
	p := &hiddenMetadataProvider{blobs: make(map[digest.Digest][]byte)}
	config := ocispec.Image{
		Platform: ocispec.Platform{OS: "linux", Architecture: "amd64"},
		RootFS:   ocispec.RootFS{Type: "layers", DiffIDs: []digest.Digest{digest.FromString("uncompressed layer")}},
	}
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.Descriptor{MediaType: ocispec.MediaTypeImageConfig},
		Layers:    []ocispec.Descriptor{{MediaType: ocispec.MediaTypeImageLayerGzip, Digest: digest.FromString("compressed layer"), Size: 123}},
	}
	if mutate != nil {
		mutate(&config, &manifest)
	}
	manifest.Config = p.add(t, config, manifest.Config.MediaType)
	target := p.add(t, manifest, manifest.MediaType)
	return p, target, manifest.Config.Digest
}

func TestResolveHiddenBytesImage(t *testing.T) {
	matcher := platforms.OnlyStrict(ocispec.Platform{OS: "linux", Architecture: "amd64"})
	for _, mode := range []string{"direct", "index", "inline", "docker"} {
		t.Run(mode, func(t *testing.T) {
			p, target, expected := hiddenMetadataFixture(t, func(_ *ocispec.Image, manifest *ocispec.Manifest) {
				if mode == "docker" {
					manifest.MediaType = images.MediaTypeDockerSchema2Manifest
					manifest.Config.MediaType = images.MediaTypeDockerSchema2Config
					manifest.Layers[0].MediaType = images.MediaTypeDockerSchema2LayerGzip
				}
			})
			if mode == "index" {
				skipped := target
				skipped.Digest = digest.FromString("not stored")
				skipped.Platform = &ocispec.Platform{OS: "linux", Architecture: "arm64"}
				target = p.add(t, ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, Manifests: []ocispec.Descriptor{skipped, target}}, ocispec.MediaTypeImageIndex)
			}
			if mode == "inline" {
				var manifest ocispec.Manifest
				require.NoError(t, json.Unmarshal(p.blobs[target.Digest], &manifest))
				manifest.Config.Data = p.blobs[expected]
				target = p.add(t, manifest, manifest.MediaType)
				target.Data = p.blobs[target.Digest]
				clear(p.blobs)
			}
			result, err := ResolveHiddenBytesImage(context.Background(), p, target, matcher, expected)
			require.NoError(t, err)
			require.Equal(t, expected, result.Manifest.Config.Digest)
			require.Equal(t, []digest.Digest{digest.FromString("uncompressed layer")}, result.DiffIDs)
			require.Equal(t, p.opened, p.closed)
			if mode == "inline" {
				require.Zero(t, p.opened)
			}
		})
	}
}

func TestResolveHiddenBytesImagePrefersNativePlatform(t *testing.T) {
	p, target, expected := hiddenMetadataFixture(t, func(config *ocispec.Image, _ *ocispec.Manifest) {
		config.Architecture = "arm64"
	})
	target.Platform = &ocispec.Platform{OS: "linux", Architecture: "arm64"}
	compatible := target
	compatible.Digest = digest.FromString("compatible manifest not pulled")
	compatible.Platform = &ocispec.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}
	matcher := platforms.Only(*target.Platform)
	require.True(t, matcher.Match(*compatible.Platform))
	index := p.add(t, ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, Manifests: []ocispec.Descriptor{compatible, target}}, ocispec.MediaTypeImageIndex)
	result, err := ResolveHiddenBytesImage(context.Background(), p, index, matcher, expected)
	require.NoError(t, err)
	require.Equal(t, expected, result.Manifest.Config.Digest)
	require.Equal(t, 3, p.opened)
	require.Equal(t, p.opened, p.closed)
}

func TestResolveHiddenBytesImageRejectsUnsupportedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*ocispec.Image, *ocispec.Manifest)
		message string
	}{
		{"platform", func(c *ocispec.Image, _ *ocispec.Manifest) { c.Architecture = "arm64" }, "no longer points to"},
		{"non Linux", func(c *ocispec.Image, _ *ocispec.Manifest) { c.OS = "windows" }, "no longer points to"},
		{"rootfs", func(c *ocispec.Image, _ *ocispec.Manifest) { c.RootFS.Type = "unknown" }, "rootfs type"},
		{"count", func(c *ocispec.Image, _ *ocispec.Manifest) { c.RootFS.DiffIDs = nil }, "layer count"},
		{"empty", func(c *ocispec.Image, m *ocispec.Manifest) { c.RootFS.DiffIDs = nil; m.Layers = nil }, "layer count"},
		{"diffID", func(c *ocispec.Image, _ *ocispec.Manifest) { c.RootFS.DiffIDs[0] = "sha999:abc" }, "invalid layer diffID"},
		{"digest", func(_ *ocispec.Image, m *ocispec.Manifest) { m.Layers[0].Digest = "invalid" }, "invalid layer digest"},
		{"negative size", func(_ *ocispec.Image, m *ocispec.Manifest) { m.Layers[0].Size = -1 }, "negative layer size"},
		{"encrypted", func(_ *ocispec.Image, m *ocispec.Manifest) { m.Layers[0].MediaType += "+encrypted" }, "layer media type"},
		{"config media", func(_ *ocispec.Image, m *ocispec.Manifest) { m.Config.MediaType = "unknown" }, "config media type"},
		{"schema", func(_ *ocispec.Image, m *ocispec.Manifest) { m.SchemaVersion = 1 }, "invalid hidden byte image manifest"},
		{"layer cap", func(c *ocispec.Image, m *ocispec.Manifest) {
			for len(m.Layers) <= hiddenBytesLayerLimit {
				m.Layers = append(m.Layers, m.Layers[0])
				c.RootFS.DiffIDs = append(c.RootFS.DiffIDs, c.RootFS.DiffIDs[0])
			}
		}, "layer count"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, target, expected := hiddenMetadataFixture(t, tc.mutate)
			result, err := ResolveHiddenBytesImage(context.Background(), p, target, platforms.OnlyStrict(ocispec.Platform{OS: "linux", Architecture: "amd64"}), expected)
			require.ErrorContains(t, err, tc.message)
			require.Empty(t, result)
			require.Equal(t, p.opened, p.closed)
		})
	}
	t.Run("moved reference", func(t *testing.T) {
		p, target, _ := hiddenMetadataFixture(t, nil)
		_, err := ResolveHiddenBytesImage(context.Background(), p, target, nil, digest.FromString("old config"))
		require.ErrorContains(t, err, "no longer points to")
		require.Equal(t, 1, p.opened)
	})
	t.Run("oversized config rejected before reading", func(t *testing.T) {
		p, target, expected := hiddenMetadataFixture(t, nil)
		var manifest ocispec.Manifest
		require.NoError(t, json.Unmarshal(p.blobs[target.Digest], &manifest))
		manifest.Config.Size = hiddenBytesMetadataBlobLimit + 1
		target = p.add(t, manifest, manifest.MediaType)
		_, err := ResolveHiddenBytesImage(context.Background(), p, target, nil, expected)
		require.ErrorContains(t, err, "blob size limit")
		require.Equal(t, 1, p.opened)
	})
}

func TestHiddenBytesMetadataReadBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*ocispec.Descriptor, *hiddenMetadataProvider)
		message string
	}{
		{"oversize descriptor", func(d *ocispec.Descriptor, _ *hiddenMetadataProvider) { d.Size = hiddenBytesMetadataBlobLimit + 1 }, "blob size limit"},
		{"negative descriptor", func(d *ocispec.Descriptor, _ *hiddenMetadataProvider) { d.Size = -1 }, "blob size limit"},
		{"stored size", func(d *ocispec.Descriptor, p *hiddenMetadataProvider) { p.blobs[d.Digest] = []byte("extra data") }, "size does not match"},
		{"inline size", func(d *ocispec.Descriptor, _ *hiddenMetadataProvider) { d.Data = []byte("extra data") }, "inline metadata size"},
		{"inline digest", func(d *ocispec.Descriptor, p *hiddenMetadataProvider) {
			d.Data = bytes.Repeat([]byte{'x'}, len(p.blobs[d.Digest]))
		}, "digest mismatch"},
		{"digest", func(d *ocispec.Descriptor, p *hiddenMetadataProvider) { p.blobs[d.Digest][0] ^= 1 }, "digest mismatch"},
		{"missing local", func(d *ocispec.Descriptor, p *hiddenMetadataProvider) {
			delete(p.blobs, d.Digest)
			d.URLs = []string{"https://must-not-fetch.invalid/blob"}
		}, "missing local blob"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &hiddenMetadataProvider{blobs: make(map[digest.Digest][]byte)}
			desc := p.add(t, "value", ocispec.MediaTypeImageConfig)
			tc.mutate(&desc, p)
			r := hiddenBytesMetadataReader{provider: p}
			_, err := r.read(context.Background(), desc)
			require.ErrorContains(t, err, tc.message)
			require.Equal(t, p.opened, p.closed)
		})
	}
	t.Run("aggregate including inline data", func(t *testing.T) {
		data := bytes.Repeat([]byte{' '}, hiddenBytesMetadataBlobLimit)
		desc := ocispec.Descriptor{Digest: digest.FromBytes(data), Size: int64(len(data)), Data: data}
		r := hiddenBytesMetadataReader{}
		for range 4 {
			_, err := r.read(context.Background(), desc)
			require.NoError(t, err)
		}
		_, err := r.read(context.Background(), desc)
		require.ErrorContains(t, err, "total size limit")
	})
	t.Run("cancellation during read closes reader", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p := &hiddenMetadataProvider{blobs: make(map[digest.Digest][]byte), onRead: cancel}
		desc := p.add(t, strings.Repeat("x", 64*1024), ocispec.MediaTypeImageConfig)
		r := hiddenBytesMetadataReader{provider: p}
		_, err := r.read(ctx, desc)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, p.closed)
	})
}

func TestHiddenBytesMetadataTraversalBounds(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		p, target, expected := hiddenMetadataFixture(t, nil)
		for range hiddenBytesMetadataDepthLimit {
			target = p.add(t, ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, Manifests: []ocispec.Descriptor{target}}, ocispec.MediaTypeImageIndex)
		}
		_, err := ResolveHiddenBytesImage(context.Background(), p, target, nil, expected)
		require.ErrorContains(t, err, "depth limit")
		require.Equal(t, p.opened, p.closed)
	})
	t.Run("descriptor count", func(t *testing.T) {
		p, target, _ := hiddenMetadataFixture(t, nil)
		var children []ocispec.Descriptor
		for range hiddenBytesMetadataVisitLimit {
			children = append(children, target)
		}
		target = p.add(t, ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, Manifests: children}, ocispec.MediaTypeImageIndex)
		_, err := ResolveHiddenBytesImage(context.Background(), p, target, nil, digest.FromString("not present"))
		require.ErrorContains(t, err, "descriptor limit")
	})
	t.Run("ancestor cycle", func(t *testing.T) {
		p, target, expected := hiddenMetadataFixture(t, nil)
		r := hiddenBytesMetadataReader{provider: p, ancestors: map[digest.Digest]bool{target.Digest: true}}
		_, _, err := r.resolve(context.Background(), target, nil, expected, 1)
		require.ErrorContains(t, err, "descriptor cycle")
		require.Zero(t, p.opened)
	})
}

func TestHiddenBytesLayerCompression(t *testing.T) {
	for _, tc := range []struct{ mediaType, compression string }{
		{ocispec.MediaTypeImageLayer, ""},
		{images.MediaTypeDockerSchema2LayerForeign, ""},
		{ocispec.MediaTypeImageLayerGzip, "gzip"},
		{images.MediaTypeDockerSchema2LayerGzip, "gzip"},
		{ocispec.MediaTypeImageLayerZstd, "zstd"},
		{images.MediaTypeDockerSchema2LayerZstd, "zstd"},
		{"application/vnd.oci.image.layer.nondistributable.v1.tar+gzip", "gzip"},
	} {
		compression, err := hiddenBytesLayerCompression(tc.mediaType)
		require.NoError(t, err)
		require.Equal(t, tc.compression, compression)
	}
	_, err := hiddenBytesLayerCompression(images.MediaTypeImageLayerEncrypted)
	require.Error(t, err)
}
