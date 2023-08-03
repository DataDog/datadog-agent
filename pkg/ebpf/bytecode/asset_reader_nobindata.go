// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ebpf_bindata

package bytecode

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
)

// GetReader returns a new AssetReader for the specified file asset
func GetReader(dir, name string) (AssetReader, error) {
	assetPath := path.Join(dir, path.Base(name))
	err := VerifyAssetPermissions(assetPath)
	if err != nil {
		return nil, err
	}

	asset, err := os.Open(assetPath)
	if err != nil {
		return nil, fmt.Errorf("could not find asset: %w", err)
	}

	return asset, nil
}

func GetCompressedReader(dir, name, archiveName string) (AssetReader, error) {
	archivePath := path.Join(dir, path.Base(archiveName))
	err := VerifyAssetPermissions(archivePath)
	if err != nil {
		return nil, err
	}

	assetPath := path.Join(dir, path.Base(name))

	if err := extractFile(archivePath, name, assetPath); err != nil {
		return nil, err
	}

	if err := os.Chown(assetPath, 0, 0); err != nil {
		return nil, err
	}

	return GetReader(dir, name)
}

func extractFile(archivePath, filename, destPath string) error {
	needleFilename := "pkg/ebpf/bytecode/build/" + filename

	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()

	archiveReader, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}
	defer archiveReader.Close()

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(0022))
	if err != nil {
		return err
	}
	defer dest.Close()

	tarReader := tar.NewReader(archiveReader)
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			return fmt.Errorf("%s not found in %s", filename, archivePath)
		}
		if err != nil {
			return err
		}

		if hdr.Name == needleFilename {
			// we found the searched file
			if _, err := io.Copy(dest, tarReader); err != nil {
				return err
			}

			// we ensure the write succeeded
			if err := dest.Sync(); err != nil {
				return err
			}

			if err := dest.Close(); err != nil {
				return err
			}

			return nil
		}
	}
}
