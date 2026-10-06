// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ebpf_bindata

package bytecode

import (
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
func GetReader(dir, name string) (AssetReader, error) {
	assetPath := path.Join(dir, path.Base(name))
	// Open and permission-check the same descriptor (O_NOFOLLOW) so there is no
	// path re-resolution between the check and the returned reader.
	asset, err := VerifyAssetPermissionsAndOpen(assetPath)
	if err != nil {
		return nil, err
	}

	return asset, nil
}
