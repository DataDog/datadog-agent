// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ebpf_bindata

package bytecode

import (
	"path"
)

// GetReader returns a new AssetReader for the specified file asset
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
