// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ebpf_bindata

package bytecode

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
)

// GetReader returns a new AssetReader for the specified file asset.
//
// This is the shared entry point for loading on-disk eBPF objects across all
// system-probe consumers (precompiled and CO-RE assets for NPM, USM, CWS and
// dynamic instrumentation), all of which run as root. Every asset is already
// required to be root-owned — VerifyAssetPermissionsAndOpen enforces that on
// the returned descriptor, as VerifyAssetPermissions did before it — so the
// only behavioral change from opening with O_NOFOLLOW is that a symlink at the
// final path component is rejected instead of followed. That is intentional and
// safe: these objects are always shipped or compiled as regular root-owned
// files (intermediate directory symlinks, e.g. a versioned install path, are
// still followed, since O_NOFOLLOW only affects the final component).
//
// Assets may also be shipped compressed, as "<name>.gz" or "<name>.xz"; the
// CWS objects in particular are ~10 MB each and shrink by 8x or more. The plain
// file wins when both are present, so an installation that ships uncompressed
// objects behaves exactly as before. Compressed assets are permission-checked
// with the same fstat-on-the-open-descriptor logic and decompressed from that
// verified descriptor, so the check-and-use-the-same-fd invariant above holds
// for them too.
func GetReader(dir, name string) (AssetReader, error) {
	assetPath := path.Join(dir, path.Base(name))
	// Open and permission-check the same descriptor (O_NOFOLLOW) so there is no
	// path re-resolution between the check and the returned reader.
	asset, err := VerifyAssetPermissionsAndOpen(assetPath)
	if err == nil {
		return asset, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	for _, ext := range compressedAssetExtensions {
		reader, compressedErr := getCompressedReader(assetPath, ext)
		if compressedErr == nil {
			return reader, nil
		}
		if !errors.Is(compressedErr, fs.ErrNotExist) {
			return nil, compressedErr
		}
	}

	// No variant of the asset exists: report the plain path, which is what
	// callers reference and what troubleshooting docs point at.
	return nil, err
}

// getCompressedReader opens and decompresses assetPath+ext. The returned error
// wraps fs.ErrNotExist when that variant simply is not installed, which lets
// the caller move on to the next candidate extension.
func getCompressedReader(assetPath, ext string) (AssetReader, error) {
	compressedPath := assetPath + ext
	compressed, err := VerifyAssetPermissionsAndOpen(compressedPath)
	if err != nil {
		return nil, err
	}
	defer compressed.Close()

	reader, err := decompressAsset(ext, compressed, maxDecompressedAssetSize)
	if err != nil {
		return nil, fmt.Errorf("error decompressing asset file %s: %w", compressedPath, err)
	}
	return reader, nil
}
