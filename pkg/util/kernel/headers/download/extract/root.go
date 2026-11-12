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
// symlinks within the destination, and later entries may resolve through them.
// os.Root confines every operation to the extraction tree, so such links can
// only redirect writes to other paths inside the tree, which the archive can
// write directly anyway. Symlink targets are the exception: os.Root confines
// where a link is created, not where it points, and later consumers follow
// links with ordinary filesystem calls. See symlink. The methods keep no
// per-archive state: parents are created only when an operation reports that
// they are missing.
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

func (destination *extractionDirectory) mkdir(name string) error {
	return destination.root.MkdirAll(name, 0755)
}

// withParents runs create, and creates the parent directories of name only if
// create reports that they are missing. Archives usually list a directory
// before its contents, so the common case costs a single operation.
func (destination *extractionDirectory) withParents(name string, create func() error) error {
	err := create()
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := destination.mkdir(filepath.Dir(name)); err != nil {
		return err
	}
	return create()
}

func (destination *extractionDirectory) write(name string, mode int64, reader io.Reader) error {
	var output *os.File
	err := destination.withParents(name, func() (err error) {
		output, err = destination.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644|os.FileMode(mode&0111))
		return err
	})
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

// symlink creates name pointing to target. linkTarget checks the target against
// the archive path of the link, which matches where the link actually lands only
// if every parent is a real directory: under a symlinked parent, a lexically
// local target such as ../sentinel could point outside the tree. Symlinks are
// rare in header packages, so checking their parents costs little.
func (destination *extractionDirectory) symlink(name, target string) error {
	target, err := linkTarget(name, target)
	if err != nil {
		return err
	}
	if err := destination.realParents(name); err != nil {
		return err
	}
	err = destination.root.Symlink(target, name)
	if errors.Is(err, fs.ErrExist) {
		// Re-extraction may meet the identical link; anything else is a conflict.
		if old, readErr := destination.root.Readlink(name); readErr == nil && old == target {
			return nil
		}
	}
	return err
}

// realParents creates the parents of name and checks that none of them is a
// symlink. MkdirAll alone would accept a parent that resolves through one.
func (destination *extractionDirectory) realParents(name string) error {
	parent := filepath.Dir(name)
	if parent == "." {
		return nil
	}
	if err := destination.mkdir(parent); err != nil {
		return err
	}
	current := ""
	for _, part := range strings.Split(parent, "/") {
		current = filepath.Join(current, part)
		info, err := destination.root.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("symlink parent %q is not a directory", current)
		}
	}
	return nil
}
