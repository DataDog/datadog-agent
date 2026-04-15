// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package extract

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/zstd"
	"github.com/xi2/xz"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

type onlyWriter struct {
	io.Writer
}

// ExtractTarball extracts the tarball in the `reader` with `filename` into `directory`.
func ExtractTarball(reader io.Reader, filename, directory string, logger types.Logger) error {
	var compressedTarReader io.Reader
	var err error
	ext := filepath.Ext(filename)
	switch ext {
	case ".xz":
		compressedTarReader, err = xz.NewReader(reader, 0)
		if err != nil {
			return fmt.Errorf("extract xz: %w", err)
		}
	case ".gz", ".tgz":
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("extract gzip: %w", err)
		}
		defer gzipReader.Close()
		compressedTarReader = gzipReader
	case ".bz2":
		compressedTarReader = bzip2.NewReader(reader)
	case ".zst":
		zstdReader := zstd.NewReader(reader)
		defer zstdReader.Close()
		compressedTarReader = zstdReader
	default:
		return fmt.Errorf("extract %s: unsupported extension %s", filename, ext)
	}

	buf := make([]byte, 50)
	tarReader := tar.NewReader(compressedTarReader)
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("read entry from tarball: %w", err)
		}

		path := filepath.Join(directory, hdr.Name)
		// logger.Debugf("Extracting %s to %s", hdr.Name, path)

		switch hdr.Typeflag {
		case tar.TypeSymlink:
			// If the symlink is to an absolute path, prefix it with the download directory
			if strings.HasPrefix(hdr.Linkname, "/") {
				_ = os.Symlink(filepath.Join(directory, hdr.Linkname), path)
				continue
			}
			// If the symlink is to a relative path, leave it be
			_ = os.Symlink(hdr.Linkname, path)
		case tar.TypeDir:
			_ = os.MkdirAll(path, 0755)
		case tar.TypeReg:
			output, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("create output file '%s': %w", path, err)
			}

			// By default, an os.File implements the io.ReaderFrom interface.
			// As a result, CopyBuffer will attempt to use the output.ReadFrom method to perform
			// the requested copy, which ends up calling the unbuffered io.Copy function & performing
			// a large number of allocations.
			// In order to force CopyBuffer to actually utilize the given buffer, we have to ensure
			// output does not implement the io.ReaderFrom interface.
			_, err = io.CopyBuffer(onlyWriter{output}, tarReader, buf)
			output.Close()
			if err != nil {
				return fmt.Errorf("uncompress file %s: %w", hdr.Name, err)
			}
		default:
			logger.Warnf("Unsupported header flag '%d' for '%s'", hdr.Typeflag, hdr.Name)
		}
	}

	return nil
}
