// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/xi2/xz"
)

// ErrStopWalk can be returned in WalkTarXZArchive to stop walking
var ErrStopWalk = errors.New("stop walk")

// WalkTarXZArchive walks the provided .tar.xz archive, calling walkFunc for each entry.
// If ErrStopWalk is returned from walkFunc, then the walk is stopped.
func WalkTarXZArchive(tarxzArchive string, walkFunc func(*tar.Reader, *tar.Header) error) error {
	f, err := os.Open(tarxzArchive)
	if err != nil {
		return err
	}
	defer f.Close()

	zr, err := xz.NewReader(f, 0)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return err
		}

		if err := walkFunc(tr, hdr); err != nil {
			if errors.Is(err, ErrStopWalk) {
				return nil
			}
			return err
		}
	}
	return nil
}

// TarXZExtractFile extracts a single file at path from the provided archive
func TarXZExtractFile(tarxzArchive string, path string, destinationDir string) error {
	found := false
	err := WalkTarXZArchive(tarxzArchive, func(tr *tar.Reader, hdr *tar.Header) error {
		if hdr.Typeflag == tar.TypeReg {
			if hdr.Name == path {
				found = true
				untarErr := untarFile(tr, hdr, destinationDir)
				if untarErr == nil {
					return ErrStopWalk
				}
				return untarErr
			}
		}
		return nil
	})
	if err == nil && !found {
		return fmt.Errorf("%s not found", path)
	}
	return err
}

// TarXZExtractAll extracts all regular files from the .tar.xz archive
func TarXZExtractAll(tarxzArchive string, destinationDir string) error {
	return WalkTarXZArchive(tarxzArchive, func(tr *tar.Reader, hdr *tar.Header) error {
		if hdr.Typeflag == tar.TypeReg {
			if err := untarFile(tr, hdr, destinationDir); err != nil {
				return err
			}
		}
		return nil
	})
}

func untarFile(tr *tar.Reader, hdr *tar.Header, destinationDir string) error {
	fpath, err := securejoin.SecureJoin(destinationDir, hdr.Name)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(fpath), 0755)
	if err != nil {
		return fmt.Errorf("mkdir %s: %w", fpath, err)
	}

	out, err := os.OpenFile(fpath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, hdr.FileInfo().Mode().Perm())
	if err != nil {
		return fmt.Errorf("open file %s: %w", fpath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, tr)
	if err != nil {
		return fmt.Errorf("copy file %s: %w", fpath, err)
	}
	return nil
}
