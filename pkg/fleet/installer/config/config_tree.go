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
// Symlinks are reproduced as symlinks and never followed, so the walk cannot leave the source
// tree — in particular it cannot descend into the experiment path through a link that points at
// it, which would copy the tree into itself.
type configTree struct {
	// sourcePath is the directory being copied, e.g. /opt/datadog-agent/etc.
	sourcePath string
	// targetPath is where the copy lands. It may already exist and must be empty if it does.
	targetPath string
}

// Copy reproduces the source tree at the target path.
func (t configTree) Copy(_ context.Context) error {
	return filepath.WalkDir(t.sourcePath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(t.sourcePath, path)
		if err != nil {
			return err
		}
		target := filepath.Join(t.targetPath, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			if err := os.MkdirAll(target, 0700); err != nil {
				return fmt.Errorf("could not create %s: %w", target, err)
			}
			return applyMetadata(target, info)
		case info.Mode()&os.ModeSymlink != 0:
			destination, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("could not read the link at %s: %w", path, err)
			}
			if err := os.Symlink(destination, target); err != nil {
				return fmt.Errorf("could not recreate the link at %s: %w", target, err)
			}
			return applyOwnership(target, info)
		case info.Mode().IsRegular():
			if err := copyRegularFile(path, target, info); err != nil {
				return err
			}
			return applyMetadata(target, info)
		default:
			// Sockets, fifos and device nodes are not configuration. The Agent recreates its own
			// sockets on start, so leaving them out of the copy is what an experiment wants.
			log.Warnf("skipping %s in the configuration copy: unsupported file type %s", path, info.Mode())
			return nil
		}
	})
}

// Discard removes the copy. It succeeds when the copy is already gone.
func (t configTree) Discard(_ context.Context) error {
	if err := os.RemoveAll(t.targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not discard %s: %w", t.targetPath, err)
	}
	return nil
}

func copyRegularFile(sourcePath, targetPath string, info fs.FileInfo) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", sourcePath, err)
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("could not create %s: %w", targetPath, err)
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		return fmt.Errorf("could not copy %s to %s: %w", sourcePath, targetPath, err)
	}
	return target.Close()
}

// applyMetadata carries the source's mode and ownership over to the copy.
func applyMetadata(path string, info fs.FileInfo) error {
	if err := os.Chmod(path, info.Mode().Perm()); err != nil {
		return fmt.Errorf("could not set the mode of %s: %w", path, err)
	}
	return applyOwnership(path, info)
}

// applyOwnership carries the source's uid and gid over to the copy.
//
// A copy made by an unprivileged process cannot change ownership; that is not fatal, because the
// files it produces are already owned by the account that will read them back. Only a privileged
// run has ownership to preserve, and only there can Lchown succeed.
func applyOwnership(path string, info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if err := os.Lchown(path, int(stat.Uid), int(stat.Gid)); err != nil {
		log.Warnf("could not set the ownership of %s: %v", path, err)
	}
	return nil
}
