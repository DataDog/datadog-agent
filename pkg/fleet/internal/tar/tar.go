// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tar provides utilities to extract tar archives
package tar

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Extract extracts a tar archive to the given destination path
//
// Note on security: This function does not currently attempt to fully mitigate zip-slip attacks.
// This is purposeful as the archive is extracted only after its SHA256 hash has been validated
// against its reference in the package catalog. This catalog is itself sent over Remote Config
// which guarantees its integrity.
func Extract(reader io.Reader, destinationPath string, maxSize int64) error {
	log.Debugf("Extracting archive to %s", destinationPath)
	tr := tar.NewReader(io.LimitReader(reader, maxSize))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read tar header: %w", err)
		}
		if header.Name == "./" {
			continue
		}

		target := filepath.Join(destinationPath, header.Name)

		// Check for directory traversal. Note that this is more of a sanity check than a security measure.
		if !strings.HasPrefix(target, filepath.Clean(destinationPath)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry %s is trying to escape the destination directory", header.Name)
		}

		// Extract element depending on its type
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("could not create directory: %w", err)
			}
		case tar.TypeReg:
			err = extractFile(target, tr, os.FileMode(header.Mode))
			if err != nil {
				return err // already wrapped
			}
		case tar.TypeSymlink:
			err = os.Symlink(header.Linkname, target)
			if err != nil {
				return fmt.Errorf("could not create symlink: %w", err)
			}
		case tar.TypeLink:
			// we currently don't support hard links in the installer
		default:
			log.Warnf("Unsupported tar entry type %d for %s", header.Typeflag, header.Name)
		}
	}

	log.Debugf("Successfully extracted archive to %s", destinationPath)
	return nil
}

// extractFile extracts a file from a tar archive.
// It is separated from extractTarGz to ensure `defer f.Close()` is called right after the file is written.
func extractFile(targetPath string, reader io.Reader, mode fs.FileMode) error {
	err := os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}
	f, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	if err != nil {
		return fmt.Errorf("could not write file: %w", err)
	}
	return nil
}

// Clean remove all files and directories in the destination path but not the destination path itself
func Clean(destinationPath string) error {
	files, err := os.ReadDir(destinationPath)
	if err != nil {
		return fmt.Errorf("could not list files in %s: %w", destinationPath, err)
	}
	for _, file := range files {
		err := os.RemoveAll(filepath.Join(destinationPath, file.Name()))
		if err != nil {
			return fmt.Errorf("could not remove %s: %w", file.Name(), err)
		}
	}
	return nil
}
