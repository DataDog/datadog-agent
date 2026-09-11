// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package safetar extracts untrusted tar streams into a dedicated directory.
package safetar

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	defaultDirectoryMode = 0o755
	defaultFileMode      = 0o644
)

// Limits bounds resources consumed while extracting a single layer.
type Limits struct {
	MaxExpandedBytes int64
	MaxFileBytes     int64
	MaxEntries       int
}

// Validate checks that all extraction limits are positive.
func (l Limits) Validate() error {
	if l.MaxExpandedBytes <= 0 {
		return errors.New("maximum expanded archive size must be positive")
	}
	if l.MaxFileBytes <= 0 {
		return errors.New("maximum archive file size must be positive")
	}
	if l.MaxEntries <= 0 {
		return errors.New("maximum archive entry count must be positive")
	}
	if l.MaxFileBytes > l.MaxExpandedBytes {
		return errors.New("maximum archive file size cannot exceed maximum expanded size")
	}
	return nil
}

// Extractor applies controlled permissions and resource limits while rejecting
// links, special files, path traversal, duplicate entries, and path collisions.
type Extractor struct {
	Limits Limits
}

// Extract writes reader below destination. destination must already exist and
// be a directory dedicated to this extraction.
func (e Extractor) Extract(ctx context.Context, reader io.Reader, destination string) error {
	if ctx == nil {
		return errors.New("archive extraction context is required")
	}
	if reader == nil {
		return errors.New("archive reader is required")
	}
	if destination == "" || !filepath.IsAbs(destination) {
		return errors.New("archive destination must be an absolute path")
	}
	if err := e.Limits.Validate(); err != nil {
		return err
	}
	info, err := os.Lstat(destination)
	if err != nil {
		return fmt.Errorf("could not inspect archive destination: %w", err)
	}
	if !info.IsDir() {
		return errors.New("archive destination is not a directory")
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return fmt.Errorf("could not open archive destination: %w", err)
	}
	defer root.Close()

	totalBytes := int64(0)
	entryCount := 0
	seen := make(map[string]byte)
	archiveReader := tar.NewReader(reader)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := archiveReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("could not read tar header: %w", err)
		}

		entryPath, skip, err := cleanEntryPath(header.Name)
		if err != nil {
			return err
		}
		if skip {
			continue
		}
		if _, found := seen[entryPath]; found {
			return fmt.Errorf("archive contains duplicate path %q", header.Name)
		}
		if err := checkParentEntries(seen, entryPath); err != nil {
			return err
		}
		entryCount++
		if entryCount > e.Limits.MaxEntries {
			return fmt.Errorf("archive exceeds the %d entry limit", e.Limits.MaxEntries)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(filepath.FromSlash(entryPath), defaultDirectoryMode); err != nil {
				return fmt.Errorf("could not create archive directory %q: %w", header.Name, err)
			}
			seen[entryPath] = tar.TypeDir
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return fmt.Errorf("archive file %q has a negative size", header.Name)
			}
			if header.Size > e.Limits.MaxFileBytes {
				return fmt.Errorf("archive file %q exceeds the %d byte limit", header.Name, e.Limits.MaxFileBytes)
			}
			if totalBytes > e.Limits.MaxExpandedBytes-header.Size {
				return fmt.Errorf("archive exceeds the %d byte expanded-size limit", e.Limits.MaxExpandedBytes)
			}
			if err := writeFile(ctx, root, entryPath, header, archiveReader); err != nil {
				return err
			}
			totalBytes += header.Size
			seen[entryPath] = tar.TypeReg
		default:
			return fmt.Errorf("archive path %q has unsupported type %d", header.Name, header.Typeflag)
		}
	}
}

func cleanEntryPath(name string) (cleaned string, skip bool, err error) {
	if name == "" {
		return "", false, errors.New("archive contains an empty path")
	}
	if strings.ContainsRune(name, '\\') {
		return "", false, fmt.Errorf("archive path %q contains a backslash", name)
	}
	cleaned = path.Clean(name)
	if cleaned == "." {
		return "", true, nil
	}
	if path.IsAbs(name) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false, fmt.Errorf("archive path %q escapes the destination", name)
	}
	if strings.ContainsRune(cleaned, 0) {
		return "", false, fmt.Errorf("archive path %q contains a NUL byte", name)
	}
	return cleaned, false, nil
}

func checkParentEntries(seen map[string]byte, entryPath string) error {
	for parent := path.Dir(entryPath); parent != "."; parent = path.Dir(parent) {
		if kind, found := seen[parent]; found && kind != tar.TypeDir {
			return fmt.Errorf("archive path %q is below non-directory path %q", entryPath, parent)
		}
	}
	if kind, found := seen[entryPath]; found && kind != tar.TypeDir {
		return fmt.Errorf("archive path %q conflicts with an existing file", entryPath)
	}
	return nil
}

func writeFile(ctx context.Context, root *os.Root, entryPath string, header *tar.Header, reader io.Reader) (returnErr error) {
	relativePath := filepath.FromSlash(entryPath)
	if err := root.MkdirAll(filepath.Dir(relativePath), defaultDirectoryMode); err != nil {
		return fmt.Errorf("could not create parent for archive file %q: %w", header.Name, err)
	}
	mode := os.FileMode(defaultFileMode)
	if header.FileInfo().Mode()&0o111 != 0 {
		mode = 0o755
	}
	file, err := root.OpenFile(relativePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("could not create archive file %q: %w", header.Name, err)
	}
	defer func() {
		returnErr = errors.Join(returnErr, file.Close())
	}()
	written, err := io.CopyN(file, contextReader{ctx: ctx, reader: reader}, header.Size)
	if err != nil {
		return fmt.Errorf("could not write archive file %q: %w", header.Name, err)
	}
	if written != header.Size {
		return fmt.Errorf("archive file %q contains %d bytes, expected %d", header.Name, written, header.Size)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
