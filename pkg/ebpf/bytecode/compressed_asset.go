// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ebpf_bindata

package bytecode

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/xi2/xz"
)

// compressedAssetExtensions lists the suffixes recognized as compressed eBPF
// assets, in the order they are probed when the plain asset is absent. Keep in
// sync with EBPF_COMPRESSION_FORMATS in tasks/system_probe.py.
//
// gzip is first on purpose: it is roughly an order of magnitude faster to
// decompress in pure Go than xz, and eBPF objects are decompressed on the
// system-probe startup path. xz is supported for installations that care more
// about on-disk footprint than about a few hundred extra milliseconds at boot
// (it is about twice as small as gzip on the CWS objects).
var compressedAssetExtensions = []string{".gz", ".xz"}

// maxDecompressedAssetSize caps how much a compressed asset is allowed to
// expand to. The largest eBPF object the Agent ships is ~10 MB, so 256 MB
// leaves plenty of headroom while keeping a corrupt or hostile archive from
// exhausting memory. Compressed assets are already required to be root-owned
// (see VerifyAssetPermissionsAndOpen), so this is defense in depth rather than
// the primary trust boundary.
const maxDecompressedAssetSize = 256 * 1024 * 1024

// decompressAsset decodes the compressed asset in r, whose format is derived
// from ext, and returns a reader over the plain bytes. Assets decompressing to
// more than maxSize bytes are rejected.
//
// The result is fully materialized in memory because the eBPF loaders parse the
// object as an ELF and therefore need random access (io.ReaderAt, see
// AssetReader): a decompressing stream cannot provide that.
func decompressAsset(ext string, r io.Reader, maxSize int64) (AssetReader, error) {
	var decompressed io.Reader

	switch ext {
	case ".gz":
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		defer gz.Close()
		decompressed = gz
	case ".xz":
		xr, err := xz.NewReader(r, 0)
		if err != nil {
			return nil, fmt.Errorf("xz: %w", err)
		}
		decompressed = xr
	default:
		return nil, fmt.Errorf("unsupported compressed asset extension %q", ext)
	}

	// Read one byte past the cap so an oversized asset is reported as an error
	// instead of being silently truncated into an unparsable ELF.
	content, err := io.ReadAll(io.LimitReader(decompressed, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxSize {
		return nil, fmt.Errorf("decompressed asset exceeds %d bytes", maxSize)
	}

	return nopCloser{bytes.NewReader(content)}, nil
}
