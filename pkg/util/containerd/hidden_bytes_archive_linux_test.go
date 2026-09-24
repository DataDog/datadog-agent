// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && containerd

package containerd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/klauspost/compress/zstd"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

type hiddenArchiveTestStore struct {
	blobs       map[digest.Digest][]byte
	infoSizes   map[digest.Digest]int64
	readers     int
	closed      int
	beforeRead  func()
	readerError error
}

func (s *hiddenArchiveTestStore) Info(_ context.Context, id digest.Digest) (content.Info, error) {
	blob, ok := s.blobs[id]
	if !ok {
		return content.Info{}, errors.New("missing local content")
	}
	size := int64(len(blob))
	if override, ok := s.infoSizes[id]; ok {
		size = override
	}
	return content.Info{Digest: id, Size: size}, nil
}

func (s *hiddenArchiveTestStore) ReaderAt(_ context.Context, descriptor ocispec.Descriptor) (content.ReaderAt, error) {
	s.readers++
	if s.readerError != nil {
		return nil, s.readerError
	}
	blob, ok := s.blobs[descriptor.Digest]
	if !ok {
		return nil, errors.New("missing local content")
	}
	return &hiddenArchiveTestReader{Reader: bytes.NewReader(blob), store: s}, nil
}

type hiddenArchiveTestReader struct {
	*bytes.Reader
	store *hiddenArchiveTestStore
}

func (r *hiddenArchiveTestReader) ReadAt(p []byte, offset int64) (int, error) {
	if r.store.beforeRead != nil {
		r.store.beforeRead()
	}
	return r.Reader.ReadAt(p, offset)
}

func (r *hiddenArchiveTestReader) Close() error {
	r.store.closed++
	return nil
}

func hiddenArchiveFile(name string, size int64) tar.Header {
	return tar.Header{Name: name, Size: size, Typeflag: tar.TypeReg, Mode: 0600}
}

func hiddenArchiveLink(name, target string) tar.Header {
	return tar.Header{Name: name, Linkname: target, Typeflag: tar.TypeLink, Mode: 0600}
}

func hiddenArchiveTar(t *testing.T, headers []tar.Header) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	for _, header := range headers {
		require.NoError(t, writer.WriteHeader(&header))
		if header.Size > 0 {
			_, err := writer.Write(bytes.Repeat([]byte{'x'}, int(header.Size)))
			require.NoError(t, err)
		}
	}
	require.NoError(t, writer.Close())
	return output.Bytes()
}

func hiddenArchiveImage(t *testing.T, compression string, layers ...[]tar.Header) (*hiddenArchiveTestStore, HiddenBytesImage) {
	t.Helper()
	store := &hiddenArchiveTestStore{blobs: make(map[digest.Digest][]byte), infoSizes: make(map[digest.Digest]int64)}
	var image HiddenBytesImage
	for _, headers := range layers {
		tarBytes := hiddenArchiveTar(t, headers)
		blob := tarBytes
		mediaType := ocispec.MediaTypeImageLayer
		switch compression {
		case "gzip":
			var output bytes.Buffer
			writer := gzip.NewWriter(&output)
			_, err := writer.Write(tarBytes)
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			blob = output.Bytes()
			mediaType = ocispec.MediaTypeImageLayerGzip
		case "zstd":
			writer, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
			require.NoError(t, err)
			blob = writer.EncodeAll(tarBytes, nil)
			require.NoError(t, writer.Close())
			mediaType = ocispec.MediaTypeImageLayerZstd
		}
		descriptor := ocispec.Descriptor{Digest: digest.FromBytes(blob), Size: int64(len(blob)), MediaType: mediaType}
		image.Manifest.Layers = append(image.Manifest.Layers, descriptor)
		image.DiffIDs = append(image.DiffIDs, digest.FromBytes(tarBytes))
		store.blobs[descriptor.Digest] = blob
	}
	return store, image
}

func TestHiddenBytesArchiveEncodingsAndSnapshotParity(t *testing.T) {
	fixtures := []hiddenFixture{
		{files: map[string]int64{"app": 4096, "dir/old": 8192}, hardlinks: map[string]string{"alias": "app"}},
		{files: map[string]int64{"app": 512, "dir/new": 10}, opaque: []string{"dir"}},
		{whiteouts: []string{"app", "alias"}},
	}
	layers, attrs := makeHiddenLayers(t, fixtures)
	want, err := calculateHiddenBytes(t.Context(), layers, DefaultHiddenBytesLimits(), attrs)
	require.NoError(t, err)
	require.Equal(t, []uint64{12288, 512, 0}, want.Counts)
	require.Equal(t, uint64(12810), want.UncompressedSize)
	for _, compression := range []string{"", "gzip", "zstd"} {
		t.Run(compression, func(t *testing.T) {
			store, image := hiddenArchiveImage(t, compression,
				[]tar.Header{hiddenArchiveFile("app", 4096), hiddenArchiveLink("alias", "app"), hiddenArchiveFile("dir/old", 8192)},
				[]tar.Header{hiddenArchiveFile("app", 512), hiddenArchiveFile("dir/new", 10), hiddenArchiveFile("dir/.wh..wh..opq", 0)},
				[]tar.Header{hiddenArchiveFile(".wh.app", 0), hiddenArchiveFile(".wh.alias", 0)},
			)
			got, err := CalculateHiddenBytesFromContent(t.Context(), store, image, DefaultHiddenBytesLimits())
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.Equal(t, len(image.DiffIDs), store.readers)
			require.Equal(t, store.readers, store.closed)
		})
	}
}

func TestHiddenBytesArchiveVisibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		layers [][]tar.Header
		want   []uint64
	}{
		{"alias survives", [][]tar.Header{{hiddenArchiveFile("a", 10), hiddenArchiveLink("b", "a")}, {hiddenArchiveFile(".wh.a", 0)}}, []uint64{0, 0}},
		{"all aliases removed", [][]tar.Header{{hiddenArchiveFile("a", 10), hiddenArchiveLink("b", "a"), hiddenArchiveLink("c", "b")}, {hiddenArchiveFile(".wh.a", 0), hiddenArchiveFile(".wh.b", 0), hiddenArchiveFile(".wh.c", 0)}}, []uint64{10, 0}},
		{"replace then remove survivor", [][]tar.Header{{hiddenArchiveFile("a", 10), hiddenArchiveLink("b", "a")}, {hiddenArchiveFile("a", 20)}, {hiddenArchiveFile(".wh.b", 0)}}, []uint64{10, 0, 0}},
		{"opaque outside alias survives", [][]tar.Header{{hiddenArchiveFile("dir/a", 10), hiddenArchiveLink("b", "dir/a")}, {hiddenArchiveFile("dir/.wh..wh..opq", 0)}}, []uint64{0, 0}},
		{"implicit parents deleted", [][]tar.Header{{hiddenArchiveFile("a/b/file", 10)}, {hiddenArchiveFile(".wh.a", 0)}}, []uint64{10, 0}},
		{"marker before new file", [][]tar.Header{{hiddenArchiveFile("a", 10)}, {hiddenArchiveFile(".wh.a", 0), hiddenArchiveFile("a", 20)}}, []uint64{10, 0}},
		{"marker after new file", [][]tar.Header{{hiddenArchiveFile("a", 10)}, {hiddenArchiveFile("a", 20), hiddenArchiveFile(".wh.a", 0)}}, []uint64{10, 0}},
		{"opaque root", [][]tar.Header{{hiddenArchiveFile("a", 10)}, {hiddenArchiveFile("b", 20), hiddenArchiveFile(".wh..wh..opq", 0)}}, []uint64{10, 0}},
		{"directory header after children", [][]tar.Header{{hiddenArchiveFile("dir/old", 10)}, {hiddenArchiveFile("dir/.wh..wh..opq", 0), hiddenArchiveFile("dir/new", 20), {Name: "dir/", Typeflag: tar.TypeDir}}}, []uint64{10, 0}},
		{"explicit directory replaces lower file", [][]tar.Header{{hiddenArchiveFile("dir", 10)}, {{Name: "dir/", Typeflag: tar.TypeDir}, hiddenArchiveFile("dir/new", 20)}}, []uint64{10, 0}},
		{"symlink replaces file", [][]tar.Header{{hiddenArchiveFile("a", 10)}, {{Name: "a", Typeflag: tar.TypeSymlink, Linkname: "/outside"}}}, []uint64{10, 0}},
		{"same sizes distinct files", [][]tar.Header{{hiddenArchiveFile("a", 10), hiddenArchiveFile("b", 10)}, {hiddenArchiveFile(".wh.a", 0), hiddenArchiveFile(".wh.b", 0)}}, []uint64{20, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, image := hiddenArchiveImage(t, "", tc.layers...)
			got, err := CalculateHiddenBytesFromContent(t.Context(), store, image, DefaultHiddenBytesLimits())
			require.NoError(t, err)
			require.Equal(t, tc.want, got.Counts)
			var total uint64
			for _, layer := range tc.layers {
				for _, header := range layer {
					if header.Typeflag == tar.TypeReg {
						total += uint64(header.Size)
					}
				}
			}
			require.Equal(t, total, got.UncompressedSize)
		})
	}
}

func TestHiddenBytesArchiveUnsupportedLayouts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		layers [][]tar.Header
	}{
		{"forward alias", [][]tar.Header{{hiddenArchiveLink("b", "a"), hiddenArchiveFile("a", 20)}}},
		{"lower alias", [][]tar.Header{{hiddenArchiveFile("a", 10)}, {hiddenArchiveLink("b", "a")}}},
		{"lower alias before replacement regression", [][]tar.Header{{hiddenArchiveFile("a", 10)}, {hiddenArchiveLink("b", "a"), hiddenArchiveFile("a", 20)}, {hiddenArchiveFile(".wh.a", 0)}}},
		{"alias cycle", [][]tar.Header{{hiddenArchiveLink("b", "a"), hiddenArchiveLink("a", "b")}}},
		{"alias to symlink", [][]tar.Header{{{Name: "a", Typeflag: tar.TypeSymlink, Linkname: "somewhere"}, hiddenArchiveLink("b", "a")}}},
		{"unsafe alias", [][]tar.Header{{hiddenArchiveFile("a", 10), hiddenArchiveLink("b", "../a")}}},
		{"absolute path", [][]tar.Header{{hiddenArchiveFile("/a", 10)}}},
		{"traversal path", [][]tar.Header{{hiddenArchiveFile("a/../b", 10)}}},
		{"duplicate normalized path", [][]tar.Header{{hiddenArchiveFile("a", 10), hiddenArchiveFile("./a", 20)}}},
		{"root file", [][]tar.Header{{hiddenArchiveFile(".", 10)}}},
		{"implicit replaces lower symlink", [][]tar.Header{{{Name: "dir", Typeflag: tar.TypeSymlink, Linkname: "/outside"}}, {hiddenArchiveFile("dir/a", 10)}}},
		{"implicit replaces lower file", [][]tar.Header{{hiddenArchiveFile("dir", 10)}, {hiddenArchiveFile("dir/a", 10)}}},
		{"non-directory ancestor", [][]tar.Header{{hiddenArchiveFile("dir", 10), hiddenArchiveFile("dir/a", 10)}}},
		{"replace implicit parent", [][]tar.Header{{hiddenArchiveFile("dir/a", 10), hiddenArchiveFile("dir", 10)}}},
		{"whiteout ancestor", [][]tar.Header{{hiddenArchiveFile(".wh.dir/a", 10)}}},
		{"whiteout with data", [][]tar.Header{{hiddenArchiveFile(".wh.a", 10)}}},
		{"whiteout directory", [][]tar.Header{{{Name: ".wh.a", Typeflag: tar.TypeDir}}}},
		{"empty whiteout", [][]tar.Header{{hiddenArchiveFile(".wh.", 0)}}},
		{"traversal whiteout", [][]tar.Header{{hiddenArchiveFile(".wh...", 0)}}},
		{"reserved whiteout", [][]tar.Header{{hiddenArchiveFile(".wh..wh.unknown", 0)}}},
		{"device", [][]tar.Header{{{Name: "device", Typeflag: tar.TypeChar}}}},
		{"fifo", [][]tar.Header{{{Name: "fifo", Typeflag: tar.TypeFifo}}}},
		{"overlay metadata", [][]tar.Header{{{Name: "a", Typeflag: tar.TypeReg, Xattrs: map[string]string{"trusted.overlay.metacopy": "y"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, image := hiddenArchiveImage(t, "", tc.layers...)
			got, err := CalculateHiddenBytesFromContent(t.Context(), store, image, DefaultHiddenBytesLimits())
			require.Error(t, err)
			require.Nil(t, got)
			require.Equal(t, store.readers, store.closed)
		})
	}
}

func TestHiddenBytesArchiveFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*hiddenArchiveTestStore, *HiddenBytesImage, *HiddenBytesLimits)
	}{
		{"missing blob", func(s *hiddenArchiveTestStore, image *HiddenBytesImage, _ *HiddenBytesLimits) {
			delete(s.blobs, image.Manifest.Layers[1].Digest)
		}},
		{"GC race", func(s *hiddenArchiveTestStore, _ *HiddenBytesImage, _ *HiddenBytesLimits) {
			s.readerError = errors.New("content was garbage collected")
		}},
		{"metadata size mismatch", func(s *hiddenArchiveTestStore, image *HiddenBytesImage, _ *HiddenBytesLimits) {
			s.infoSizes[image.Manifest.Layers[0].Digest] = 1
		}},
		{"reader size mismatch", func(s *hiddenArchiveTestStore, image *HiddenBytesImage, _ *HiddenBytesLimits) {
			id := image.Manifest.Layers[0].Digest
			s.infoSizes[id] = int64(len(s.blobs[id]))
			s.blobs[id] = s.blobs[id][:len(s.blobs[id])-1]
		}},
		{"compressed digest mismatch", func(s *hiddenArchiveTestStore, image *HiddenBytesImage, _ *HiddenBytesLimits) {
			s.blobs[image.Manifest.Layers[0].Digest][512] ^= 1
		}},
		{"expanded digest mismatch", func(_ *hiddenArchiveTestStore, image *HiddenBytesImage, _ *HiddenBytesLimits) {
			image.DiffIDs[0] = digest.FromString("different")
		}},
		{"layer count mismatch", func(_ *hiddenArchiveTestStore, image *HiddenBytesImage, _ *HiddenBytesLimits) { image.DiffIDs = nil }},
		{"unsupported media", func(_ *hiddenArchiveTestStore, image *HiddenBytesImage, _ *HiddenBytesLimits) {
			image.Manifest.Layers[0].MediaType = "application/unsupported"
		}},
		{"invalid digest", func(_ *hiddenArchiveTestStore, image *HiddenBytesImage, _ *HiddenBytesLimits) {
			image.DiffIDs[0] = "invalid"
		}},
		{"entries limit", func(_ *hiddenArchiveTestStore, _ *HiddenBytesImage, limits *HiddenBytesLimits) { limits.MaxEntries = 1 }},
		{"paths limit", func(_ *hiddenArchiveTestStore, _ *HiddenBytesImage, limits *HiddenBytesLimits) {
			limits.MaxPathBytes = 1
		}},
		{"compressed limit", func(_ *hiddenArchiveTestStore, _ *HiddenBytesImage, limits *HiddenBytesLimits) {
			limits.MaxArchiveBytes = 100
		}},
		{"zero archive limit", func(_ *hiddenArchiveTestStore, _ *HiddenBytesImage, limits *HiddenBytesLimits) {
			limits.MaxArchiveBytes = 0
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, image := hiddenArchiveImage(t, "", []tar.Header{hiddenArchiveFile("a", 10)}, []tar.Header{hiddenArchiveFile(".wh.a", 0)})
			limits := DefaultHiddenBytesLimits()
			tc.mutate(store, &image, &limits)
			got, err := CalculateHiddenBytesFromContent(t.Context(), store, image, limits)
			require.Error(t, err)
			require.Nil(t, got)
			if tc.name == "missing blob" {
				require.Zero(t, store.readers, "preflight must check every blob before opening any")
			}
			if tc.name != "GC race" {
				require.Equal(t, store.readers, store.closed)
			}
		})
	}
}

func TestHiddenBytesArchiveStreamBounds(t *testing.T) {
	for _, compression := range []string{"gzip", "zstd"} {
		t.Run(compression, func(t *testing.T) {
			store, image := hiddenArchiveImage(t, compression, []tar.Header{hiddenArchiveFile("a", 16384)})
			limits := DefaultHiddenBytesLimits()
			limits.MaxArchiveBytes = 8192
			require.Less(t, image.Manifest.Layers[0].Size, int64(limits.MaxArchiveBytes))
			got, err := CalculateHiddenBytesFromContent(t.Context(), store, image, limits)
			require.ErrorContains(t, err, "byte limit")
			require.Nil(t, got)
			require.Equal(t, store.readers, store.closed)
		})
	}
	for _, duringRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "already canceled", true: "cancel during body"}[duringRead], func(t *testing.T) {
			store, image := hiddenArchiveImage(t, "gzip", []tar.Header{hiddenArchiveFile("a", 16384)})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if duringRead {
				store.beforeRead = cancel
			} else {
				cancel()
			}
			got, err := CalculateHiddenBytesFromContent(ctx, store, image, DefaultHiddenBytesLimits())
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, got)
			require.Equal(t, store.readers, store.closed)
		})
	}
}

func TestHiddenBytesArchiveTrailingDataAndTruncation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		compression string
		mutate      func([]byte) []byte
		valid       bool
	}{
		{"zero padding", "", func(blob []byte) []byte { return append(blob, make([]byte, 512)...) }, true},
		{"nonzero trailing data", "", func(blob []byte) []byte { return append(blob, []byte("hidden extra archive")...) }, false},
		{"truncated body", "", func(blob []byte) []byte { return blob[:515] }, false},
		{"truncated gzip trailer", "gzip", func(blob []byte) []byte { return blob[:len(blob)-4] }, false},
		{"invalid gzip checksum", "gzip", func(blob []byte) []byte { blob[len(blob)-8] ^= 1; return blob }, false},
		{"truncated zstd", "zstd", func(blob []byte) []byte { return blob[:len(blob)-4] }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, image := hiddenArchiveImage(t, tc.compression, []tar.Header{hiddenArchiveFile("a", 100)})
			descriptor := &image.Manifest.Layers[0]
			blob := tc.mutate(store.blobs[descriptor.Digest])
			descriptor.Digest, descriptor.Size = digest.FromBytes(blob), int64(len(blob))
			store.blobs[descriptor.Digest] = blob
			if tc.compression == "" {
				image.DiffIDs[0] = descriptor.Digest
			}
			got, err := CalculateHiddenBytesFromContent(t.Context(), store, image, DefaultHiddenBytesLimits())
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, []uint64{0}, got.Counts)
			} else {
				require.Error(t, err)
				require.Nil(t, got)
			}
			require.Equal(t, store.readers, store.closed)
		})
	}
}

func TestHiddenBytesArchiveRejectsSparseMetadata(t *testing.T) {
	for _, key := range []string{"GNU.sparse.map", "GNU.sparse.major", "SCHILY.realsize"} {
		header := hiddenArchiveFile("sparse", 10)
		header.PAXRecords = map[string]string{key: "1"}
		require.Error(t, validateHiddenArchiveHeader(&header))
	}
	header := hiddenArchiveFile("sparse", 10)
	header.Typeflag = tar.TypeGNUSparse
	require.Error(t, validateHiddenArchiveHeader(&header))
}

func TestHiddenBytesArchiveBoundAtEOF(t *testing.T) {
	store, image := hiddenArchiveImage(t, "", []tar.Header{hiddenArchiveFile("a", 10)})
	limits := DefaultHiddenBytesLimits()
	limits.MaxArchiveBytes = uint64(image.Manifest.Layers[0].Size)
	got, err := CalculateHiddenBytesFromContent(t.Context(), store, image, limits)
	require.NoError(t, err)
	require.Equal(t, []uint64{0}, got.Counts)
	var used uint64
	reader := &hiddenArchiveReader{ctx: t.Context(), input: bytes.NewReader([]byte("ab")), used: &used, limit: 1}
	_, err = io.ReadAll(reader)
	require.ErrorContains(t, err, "byte limit")
}
