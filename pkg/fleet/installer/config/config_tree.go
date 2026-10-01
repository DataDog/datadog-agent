// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// configTree copies a configuration directory.
//
// Ownership and modes are carried over numerically, from the source file's own uid and gid, not
// by looking an account name up: the copy must reproduce what is already on the host, whatever
// that is, and a name lookup would substitute this build's idea of the Agent account for it.
//
// Every file operation goes through an os.Root opened on the source or the target, and symlinks
// are reproduced as symlinks and never followed, so the copy cannot read or write outside the two
// trees — in particular it cannot descend into the experiment path through a link that points at
// it, which would copy the tree into itself.
type configTree struct {
	// sourcePath is the directory being copied, e.g. /opt/datadog-agent/etc.
	sourcePath string
	// targetPath is where the copy lands. It may already exist and must be empty if it does.
	targetPath string
}

// Copy reproduces the source tree at the target path.
//
// The target root keeps the root-only mode it is created with: it takes on the source root's mode
// and ownership only in Finish, once nothing more will be written into the copy.
func (t configTree) Copy(_ context.Context) error {
	if err := os.Mkdir(t.targetPath, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("could not create %s: %w", t.targetPath, err)
	}
	source, err := os.OpenRoot(t.sourcePath)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", t.sourcePath, err)
	}
	defer source.Close()
	target, err := os.OpenRoot(t.targetPath)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", t.targetPath, err)
	}
	defer target.Close()

	return fs.WalkDir(source.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case path == ".":
			return nil
		case entry.IsDir():
			if err := target.Mkdir(path, 0700); err != nil {
				return fmt.Errorf("could not create %s: %w", path, err)
			}
			return applyDirectoryMetadata(target, path, info)
		case info.Mode()&os.ModeSymlink != 0:
			destination, err := source.Readlink(path)
			if err != nil {
				return fmt.Errorf("could not read the link at %s: %w", path, err)
			}
			if err := target.Symlink(destination, path); err != nil {
				return fmt.Errorf("could not recreate the link at %s: %w", path, err)
			}
			if uid, gid, ok := ownership(info); ok {
				if err := target.Lchown(path, uid, gid); err != nil {
					log.Warnf("could not set the ownership of %s: %v", path, err)
				}
			}
			return nil
		case info.Mode().IsRegular():
			return copyRegularFile(source, target, path)
		default:
			// Sockets, fifos and device nodes are not configuration. The Agent recreates its own
			// sockets on start, so leaving them out of the copy is what an experiment wants.
			log.Warnf("skipping %s in the configuration copy: unsupported file type %s", path, info.Mode())
			return nil
		}
	})
}

// Finish gives the target root the source root's mode and ownership.
func (t configTree) Finish(_ context.Context) error {
	source, err := os.OpenRoot(t.sourcePath)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", t.sourcePath, err)
	}
	defer source.Close()
	info, err := source.Lstat(".")
	if err != nil {
		return fmt.Errorf("could not stat %s: %w", t.sourcePath, err)
	}
	target, err := os.OpenRoot(t.targetPath)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", t.targetPath, err)
	}
	defer target.Close()
	return applyDirectoryMetadata(target, ".", info)
}

// Discard removes the copy. It succeeds when the copy is already gone.
func (t configTree) Discard(_ context.Context) error {
	parent, err := os.OpenRoot(filepath.Dir(t.targetPath))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not discard %s: %w", t.targetPath, err)
	}
	defer parent.Close()
	if err := parent.RemoveAll(filepath.Base(t.targetPath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not discard %s: %w", t.targetPath, err)
	}
	return nil
}

// copyRegularFile copies one regular file, taking its mode and ownership from the file it actually
// opened rather than from the walk, which may have seen a different entry under the same name.
func copyRegularFile(source, target *os.Root, path string) error {
	// O_NONBLOCK keeps a fifo swapped in under this name from blocking the open; the type check
	// below then rejects it.
	in, err := source.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", path, err)
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("could not stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is no longer a regular file", path)
	}
	out, err := target.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return fmt.Errorf("could not create %s: %w", path, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("could not copy %s: %w", path, err)
	}
	if err := applyMetadata(out, info); err != nil {
		return err
	}
	return out.Close()
}

// applyDirectoryMetadata carries a source directory's mode and ownership over to the copy, through
// a handle that refuses to resolve to anything but the directory itself.
func applyDirectoryMetadata(target *os.Root, path string, info fs.FileInfo) error {
	dir, err := target.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", path, err)
	}
	defer dir.Close()
	return applyMetadata(dir, info)
}

// applyMetadata carries the source's mode and ownership over to an open copy.
//
// A copy made by an unprivileged process cannot change ownership; that is not fatal, because the
// files it produces are already owned by the account that will read them back. Only a privileged
// run has ownership to preserve, and only there can Chown succeed.
func applyMetadata(file *os.File, info fs.FileInfo) error {
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("could not set the mode of %s: %w", file.Name(), err)
	}
	if uid, gid, ok := ownership(info); ok {
		if err := file.Chown(uid, gid); err != nil {
			log.Warnf("could not set the ownership of %s: %v", file.Name(), err)
		}
	}
	return nil
}

// ownership returns the uid and gid a file is owned by.
func ownership(info fs.FileInfo) (int, int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(stat.Uid), int(stat.Gid), true
}
