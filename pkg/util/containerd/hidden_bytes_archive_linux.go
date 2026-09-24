// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && containerd

package containerd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/containerd/containerd/v2/core/content"
	"github.com/klauspost/compress/zstd"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// CalculateHiddenBytesFromContent reads only locally available layer archives.
// Unlike snapshot scanning, it streams file bodies to verify layer integrity,
// but never extracts files, reads registries, or returns partial results.
func CalculateHiddenBytesFromContent(ctx context.Context, store content.InfoReaderProvider, image HiddenBytesImage, limits HiddenBytesLimits) (*HiddenBytesResult, error) {
	if limits.MaxArchiveBytes == 0 {
		return nil, errors.New("archive byte limit must be positive")
	}
	if len(image.Manifest.Layers) != len(image.DiffIDs) || len(image.DiffIDs) > 4096 {
		return nil, errors.New("invalid archive layer count")
	}
	state, err := newHiddenBytesState(ctx, len(image.DiffIDs), limits)
	if err != nil {
		return nil, err
	}
	var declared uint64
	for i, descriptor := range image.Manifest.Layers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if descriptor.Digest.Validate() != nil || !descriptor.Digest.Algorithm().Available() || image.DiffIDs[i].Validate() != nil || !image.DiffIDs[i].Algorithm().Available() {
			return nil, errors.New("invalid archive digest")
		}
		if _, err := hiddenBytesLayerCompression(descriptor.MediaType); err != nil {
			return nil, err
		}
		if descriptor.Size < 0 || uint64(descriptor.Size) > limits.MaxArchiveBytes-declared {
			return nil, errors.New("compressed archive byte limit exceeded")
		}
		declared += uint64(descriptor.Size)
		info, err := store.Info(ctx, descriptor.Digest)
		if err != nil {
			return nil, fmt.Errorf("local layer unavailable: %w", err)
		}
		if info.Size != descriptor.Size || info.Digest != descriptor.Digest {
			return nil, errors.New("local layer metadata mismatch")
		}
	}
	var compressed, expanded uint64
	for i, descriptor := range image.Manifest.Layers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := readHiddenBytesArchive(ctx, store, descriptor, image.DiffIDs[i], i, state, &compressed, &expanded)
		if err != nil {
			return nil, fmt.Errorf("read image layer %d: %w", i, err)
		}
		if err := state.applyLayer(entries); err != nil {
			return nil, err
		}
	}
	return state.result(), nil
}

// Both byte counters span all layers. Check cancellation even while tar skips bodies.
type hiddenArchiveReader struct {
	ctx   context.Context
	input io.Reader
	used  *uint64
	limit uint64
}

func (r *hiddenArchiveReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	remaining := r.limit - *r.used
	if uint64(len(p)) > remaining {
		// Read at most one extra byte to distinguish EOF at the limit.
		p = p[:remaining+1]
	}
	n, err := r.input.Read(p)
	if uint64(n) > remaining {
		return 0, errors.New("archive byte limit exceeded")
	}
	*r.used += uint64(n)
	return n, err
}

func readHiddenBytesArchive(ctx context.Context, store content.Provider, descriptor ocispec.Descriptor, diffID digest.Digest, layer int, state *hiddenBytesState, compressed, expanded *uint64) ([]hiddenEntry, error) {
	reader, err := store.ReaderAt(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	if reader.Size() != descriptor.Size {
		return nil, errors.New("local layer size mismatch")
	}
	compressedHash := descriptor.Digest.Algorithm().Digester()
	raw := io.TeeReader(&hiddenArchiveReader{ctx: ctx, input: io.NewSectionReader(reader, 0, descriptor.Size), used: compressed, limit: state.limits.MaxArchiveBytes}, compressedHash.Hash())
	var decoded io.Reader = raw
	compression, err := hiddenBytesLayerCompression(descriptor.MediaType)
	if err != nil {
		return nil, err
	}
	switch compression {
	case "gzip":
		decoder, err := gzip.NewReader(raw)
		if err != nil {
			return nil, err
		}
		defer decoder.Close()
		decoded = decoder
	case "zstd":
		decoder, err := zstd.NewReader(raw, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderMaxWindow(64<<20))
		if err != nil {
			return nil, err
		}
		defer decoder.Close()
		decoded = decoder
	}
	expandedHash := diffID.Algorithm().Digester()
	stream := io.TeeReader(&hiddenArchiveReader{ctx: ctx, input: decoded, used: expanded, limit: state.limits.MaxArchiveBytes}, expandedHash.Hash())
	entries, err := parseHiddenBytesArchive(stream, layer, state)
	if err != nil {
		return nil, err
	}
	// archive/tar stops at the end marker, before compression trailers and padding.
	if _, err := io.Copy(hiddenArchivePadding{}, stream); err != nil {
		return nil, err
	}
	if _, err := io.Copy(io.Discard, raw); err != nil {
		return nil, err
	}
	if compressedHash.Digest() != descriptor.Digest || expandedHash.Digest() != diffID {
		return nil, errors.New("archive digest mismatch")
	}
	return entries, nil
}

type hiddenArchivePadding struct{}

func (hiddenArchivePadding) Write(p []byte) (int, error) {
	for _, b := range p {
		if b != 0 {
			return 0, errors.New("nonzero archive trailing data")
		}
	}
	return len(p), nil
}

func hiddenArchivePath(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsRune(name, 0) {
		return "", errors.New("unsafe archive path")
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", errors.New("archive path traversal")
		}
	}
	return path.Clean(name), nil
}

func parseHiddenBytesArchive(stream io.Reader, layer int, state *hiddenBytesState) ([]hiddenEntry, error) {
	var entries []hiddenEntry
	nodes := make(map[string]int)
	headers := make(map[string]struct{})
	parents := func(name string) error {
		var missing []string
		for {
			if index, exists := nodes[name]; exists {
				if !entries[index].mode.IsDir() {
					return errors.New("non-directory archive ancestor")
				}
				break
			}
			if strings.HasPrefix(path.Base(name), ".wh.") {
				return errors.New("whiteout used as archive ancestor")
			}
			if err := state.trackEntry(name); err != nil {
				return err
			}
			missing = append(missing, name)
			if name == "." {
				break
			}
			name = path.Dir(name)
		}
		for i := len(missing) - 1; i >= 0; i-- {
			name := missing[i]
			nodes[name] = len(entries)
			entries = append(entries, hiddenEntry{path: name, mode: fs.ModeDir, implicit: true})
		}
		return nil
	}
	archive := tar.NewReader(stream)
	for {
		if err := state.ctx.Err(); err != nil {
			return nil, err
		}
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}
		if err := validateHiddenArchiveHeader(header); err != nil {
			return nil, err
		}
		name, err := hiddenArchivePath(header.Name)
		if err != nil {
			return nil, err
		}
		if _, duplicate := headers[name]; duplicate {
			return nil, errors.New("duplicate archive path")
		}
		if err := state.trackEntry(name); err != nil {
			return nil, err
		}
		headers[name] = struct{}{}
		if err := parents(path.Dir(name)); err != nil {
			return nil, err
		}
		base := path.Base(name)
		if strings.HasPrefix(base, ".wh.") {
			if header.Typeflag != tar.TypeReg || header.Size != 0 {
				return nil, errors.New("invalid archive whiteout")
			}
			if base == ".wh..wh..opq" {
				entries[nodes[path.Dir(name)]].opaque = true
			} else {
				target := strings.TrimPrefix(base, ".wh.")
				if target == "" || target == "." || target == ".." || strings.HasPrefix(target, ".wh.") {
					return nil, errors.New("invalid archive whiteout name")
				}
				target = path.Join(path.Dir(name), target)
				if err := state.trackEntry(target); err != nil {
					return nil, err
				}
				entries = append(entries, hiddenEntry{path: target, whiteout: true})
			}
			continue
		}
		entry := hiddenEntry{path: name}
		switch header.Typeflag {
		case tar.TypeReg:
			entry.data, err = state.newData(uint64(header.Size), layer)
			if err != nil {
				return nil, err
			}
		case tar.TypeDir:
			entry.mode = fs.ModeDir
		case tar.TypeSymlink:
			entry.mode = fs.ModeSymlink
		case tar.TypeLink:
			target, err := hiddenArchivePath(header.Linkname)
			if err != nil {
				return nil, err
			}
			index, exists := nodes[target]
			if !exists || entries[index].data == nil {
				return nil, errors.New("hardlink target is not an earlier regular file in this layer")
			}
			entry.data = entries[index].data
		}
		if name == "." && !entry.mode.IsDir() {
			return nil, errors.New("archive root is not a directory")
		}
		if index, exists := nodes[name]; exists {
			if !entries[index].implicit || !entry.mode.IsDir() {
				return nil, errors.New("archive entry replaces a parent directory")
			}
			entry.opaque = entries[index].opaque
			entries[index] = entry
		} else {
			nodes[name] = len(entries)
			entries = append(entries, entry)
		}
		if _, err := io.Copy(io.Discard, archive); err != nil {
			return nil, err
		}
	}
}

func validateHiddenArchiveHeader(header *tar.Header) error {
	if header.Size < 0 {
		return errors.New("negative archive file size")
	}
	switch header.Typeflag {
	case tar.TypeReg:
	case tar.TypeDir, tar.TypeSymlink, tar.TypeLink:
		if header.Size != 0 {
			return errors.New("non-regular archive entry has data")
		}
	default:
		return errors.New("unsupported archive entry type")
	}
	for key := range header.PAXRecords {
		if strings.HasPrefix(key, "GNU.sparse.") || key == "SCHILY.realsize" || strings.HasPrefix(key, "SCHILY.xattr.trusted.overlay.") || strings.HasPrefix(key, "SCHILY.xattr.user.overlay.") {
			return errors.New("unsupported archive sparse or overlay metadata")
		}
	}
	return nil
}
