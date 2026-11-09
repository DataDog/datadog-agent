// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package extract is utilities for package extraction
package extract

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

type onlyWriter struct {
	io.Writer
}

// ExtractTarball extracts the tarball in the `reader` with `filename` into `directory`.
func ExtractTarball(reader io.Reader, filename, directory string, _ types.Logger) error {
	var compression compressionFormat
	extension := filepath.Ext(filename)
	switch extension {
	case ".xz":
		compression = compressionXZ
	case ".gz", ".tgz":
		compression = compressionGzip
	case ".bz2":
		compression = compressionBzip2
	case ".zst":
		compression = compressionZstd
	default:
		return fmt.Errorf("extract %s: unsupported extension %s", filename, extension)
	}
	decompressedReader, err := openCompressed(reader, compression)
	if err != nil {
		return fmt.Errorf("extract %s: %w", filename, err)
	}
	defer decompressedReader.Close()

	destination, err := openExtractionDirectory(directory)
	if err != nil {
		return err
	}
	defer destination.root.Close()
	tarReader := tar.NewReader(decompressedReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("read entry from tarball: %w", err)
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			// Global PAX records are metadata and do not create filesystem entries.
			continue
		}
		name, err := archivePath(header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = destination.mkdir(name)
		case tar.TypeReg:
			err = destination.write(name, header.Mode, tarReader)
		case tar.TypeSymlink:
			err = destination.symlink(name, header.Linkname)
		case tar.TypeLink:
			err = errors.New("tar hardlinks are unsupported")
		default:
			err = fmt.Errorf("unsupported tar entry type %d", header.Typeflag)
		}
		if err != nil {
			return fmt.Errorf("extract tar entry %q: %w", header.Name, err)
		}
	}
	// Consume the compression trailer so checksum errors are not hidden by tar EOF.
	_, err = io.CopyBuffer(io.Discard, decompressedReader, destination.buffer)
	return err
}
