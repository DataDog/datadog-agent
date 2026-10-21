// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package extract

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const copyBufferSize = 32 * 1024

// extractionDirectory provides filesystem operations shared by the tar and CPIO
// parsers. The caller provides a prepared destination; these methods do not
// decide whether its parent directory is trusted. Archive entries can create
// symlinks within the destination, so later entries must not follow those
// links to write elsewhere. os.Root confines operations to the extraction
// tree, and the methods below require archive-created parents to be real
// directories.
type extractionDirectory struct {
	root   *os.Root
	buffer []byte
}

func openExtractionDirectory(directory string) (*extractionDirectory, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &extractionDirectory{root: root, buffer: make([]byte, copyBufferSize)}, nil
}

// archivePath maps package installation paths into the extraction directory:
// /usr/src/headers and ./usr/src/headers both become usr/src/headers. Reject any
// ".." component before cleaning so an entry such as usr/../outside is refused
// rather than silently rewritten to a different destination.
func archivePath(name string) (string, error) {
	name = strings.TrimLeft(name, "/")
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", fmt.Errorf("archive path contains parent traversal: %q", name)
		}
	}
	name = filepath.Clean(name)
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("invalid archive path %q", name)
	}
	return name, nil
}

// mkdir checks each component because MkdirAll follows existing directory
// symlinks. Even a link pointing inside the destination is rejected: later archive
// entries must not redirect writes through a previously created link.
func (destination *extractionDirectory) mkdir(name string) error {
	current := ""
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		if err := destination.root.Mkdir(current, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		fi, err := destination.root.Lstat(current)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("archive parent %q is not a directory", current)
		}
	}
	return nil
}

func (destination *extractionDirectory) write(name string, mode int64, reader io.Reader) error {
	if err := destination.mkdir(filepath.Dir(name)); err != nil {
		return err
	}
	if fi, err := destination.root.Lstat(name); err == nil {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("refusing to overwrite non-regular archive entry %q", name)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	output, err := destination.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644|os.FileMode(mode&0111))
	if err != nil {
		return err
	}
	// Hide os.File.ReadFrom so io.CopyBuffer uses our reusable buffer instead
	// of delegating to the file's copy implementation.
	_, copyErr := io.CopyBuffer(onlyWriter{output}, reader, destination.buffer)
	return errors.Join(copyErr, output.Close())
}

// linkTarget accepts only leading parent components in relative targets. A parent
// after a named component could change meaning if that component became a symlink.
func linkTarget(name, target string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("empty symlink target for %q", name)
	}
	if filepath.IsAbs(target) {
		installation, err := archivePath(target)
		if err != nil {
			return "", err
		}
		return filepath.Rel(filepath.Dir(name), installation)
	}
	named := false
	for _, part := range strings.Split(target, "/") {
		switch part {
		case "", ".":
		case "..":
			if named {
				return "", fmt.Errorf("ambiguous parent traversal in symlink target %q", target)
			}
		default:
			named = true
		}
	}
	if !filepath.IsLocal(filepath.Join(filepath.Dir(name), target)) {
		return "", fmt.Errorf("symlink %q escapes extraction root: %q", name, target)
	}
	return target, nil
}

func (destination *extractionDirectory) symlink(name, target string) error {
	target, err := linkTarget(name, target)
	if err != nil {
		return err
	}
	if err := destination.mkdir(filepath.Dir(name)); err != nil {
		return err
	}
	if old, err := destination.root.Readlink(name); err != nil || old != target {
		if err := destination.root.Symlink(target, name); err != nil {
			return err
		}
	}
	// Missing targets are normal when another package supplies them later. Other
	// errors, including cycles and traversal outside the root, reject the archive.
	if _, err := destination.root.Stat(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
