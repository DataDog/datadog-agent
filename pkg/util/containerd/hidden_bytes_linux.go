// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && containerd

package containerd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// HiddenBytesLimits bounds work and path storage across an entire image scan.
type HiddenBytesLimits struct {
	MaxEntries   uint64
	MaxPathBytes uint64
}

// DefaultHiddenBytesLimits returns the maximum supported image scan size.
func DefaultHiddenBytesLimits() HiddenBytesLimits {
	return HiddenBytesLimits{MaxEntries: 500000, MaxPathBytes: 64 << 20}
}

type hiddenEntry struct {
	path     string
	size     uint64
	layer    int
	mode     fs.FileMode
	whiteout bool
	opaque   bool
}

type overlayMetadata struct {
	whiteout bool
	opaque   bool
}

type overlayMetadataReader func(string, bool) (overlayMetadata, error)

// CalculateHiddenBytes counts logical regular-file bytes hidden by later layers.
// It never opens image regular files and returns no partial results on failure.
// Reading trusted overlay metadata requires SYS_ADMIN in the initial user namespace.
func CalculateHiddenBytes(ctx context.Context, layers []ImageLayer, limits HiddenBytesLimits) ([]uint64, error) {
	if err := checkOverlayMetadataAccess(); err != nil {
		return nil, err
	}
	return calculateHiddenBytes(ctx, layers, limits, readOverlayMetadata)
}

func checkOverlayMetadataAccess() error {
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capget(&header, &data[0]); err != nil {
		return fmt.Errorf("read overlay metadata capabilities: %w", err)
	}
	if data[0].Effective&(1<<unix.CAP_SYS_ADMIN) == 0 {
		return errors.New("hidden bytes requires CAP_SYS_ADMIN to read trusted overlay metadata")
	}
	// A namespaced capability cannot prove visibility of the host's trusted xattrs.
	// Linux reserves PROC_USER_INIT_INO for the initial user namespace. Fail
	// closed if a kernel does not expose this identity: even a full identity
	// uid_map can belong to a child namespace with invisible trusted xattrs.
	namespace, err := os.Stat("/proc/self/ns/user")
	if err != nil {
		return fmt.Errorf("read user namespace identity: %w", err)
	}
	stat, ok := namespace.Sys().(*syscall.Stat_t)
	if !ok || !isInitialUserNamespace(stat.Ino) {
		return errors.New("hidden bytes requires the Linux initial user namespace")
	}
	uidMap, err := os.ReadFile("/proc/self/uid_map")
	if err != nil {
		return fmt.Errorf("read user namespace mapping: %w", err)
	}
	fields := strings.Fields(string(uidMap))
	if len(fields) != 3 || fields[0] != "0" || fields[1] != "0" || fields[2] != "4294967295" {
		return errors.New("hidden bytes does not support a remapped user namespace")
	}
	return nil
}

func isInitialUserNamespace(inode uint64) bool {
	const initialUserNamespaceInode = 0xEFFFFFFD
	return inode == initialUserNamespaceInode
}

func readOverlayMetadata(path string, userXAttr bool) (overlayMetadata, error) {
	var result overlayMetadata
	var value [256]byte
	prefix := "trusted.overlay."
	if userXAttr {
		prefix = "user.overlay."
	}
	for _, name := range []string{"opaque", "whiteout", "metacopy", "redirect"} {
		n, err := unix.Lgetxattr(path, prefix+name, value[:])
		if errors.Is(err, unix.ENODATA) {
			continue
		}
		if err != nil {
			return result, fmt.Errorf("read %s metadata on %s: %w", name, path, err)
		}
		switch name {
		case "metacopy", "redirect":
			return result, fmt.Errorf("unsupported overlay %s on %s", name, path)
		case "whiteout":
			result.whiteout = true
		case "opaque":
			switch string(value[:n]) {
			case "y":
				result.opaque = true
			case "x", "n": // x is a whiteout lookup optimization, not opacity.
			default:
				return result, fmt.Errorf("unsupported opaque metadata on %s", path)
			}
		}
	}
	return result, nil
}

func calculateHiddenBytes(ctx context.Context, layers []ImageLayer, limits HiddenBytesLimits, metadata overlayMetadataReader) ([]uint64, error) {
	if limits.MaxEntries == 0 || limits.MaxPathBytes == 0 {
		return nil, errors.New("hidden byte scan limits must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	counts := make([]uint64, len(layers))
	visible := make(map[string]hiddenEntry)
	var entries, pathBytes uint64
	reserve := func(path string) error {
		if uint64(len(path)) > limits.MaxPathBytes-pathBytes {
			return errors.New("hidden byte scan path memory limit exceeded")
		}
		pathBytes += uint64(len(path))
		return nil
	}
	remove := func(path string, old hiddenEntry) error {
		if old.mode.IsRegular() {
			if old.size > math.MaxUint64-counts[old.layer] {
				return errors.New("hidden byte count overflow")
			}
			counts[old.layer] += old.size
		}
		delete(visible, path)
		pathBytes -= uint64(len(path))
		return nil
	}
	hide := func(path string, childrenOnly bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if path != "." {
			old, exists := visible[path]
			if !exists {
				return nil
			}
			if !old.mode.IsDir() {
				if childrenOnly {
					return nil
				}
				return remove(path, old)
			}
		}
		for key, old := range visible {
			if err := ctx.Err(); err != nil {
				return err
			}
			if (key == path && !childrenOnly) || strings.HasPrefix(key, path+"/") || path == "." {
				if err := remove(key, old); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for layerIndex, layer := range layers {
		var current []hiddenEntry
		var visit func(string) error
		visit = func(relative string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries++
			if entries > limits.MaxEntries {
				return errors.New("hidden byte scan entry limit exceeded")
			}
			if err := reserve(relative); err != nil {
				return err
			}
			path := filepath.Join(layer.Path, relative)
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if relative == "." && !info.IsDir() {
				return errors.New("image snapshot root is not a directory")
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || info.Size() < 0 {
				return errors.New("unsupported image file metadata")
			}
			if info.Mode().IsRegular() && stat.Nlink != 1 {
				return fmt.Errorf("hardlinked image file is unsupported: %s", relative)
			}
			meta, err := metadata(path, layer.UserXAttr)
			if err != nil {
				return err
			}
			nativeWhiteout := info.Mode()&os.ModeCharDevice != 0 && unix.Major(uint64(stat.Rdev)) == 0 && unix.Minor(uint64(stat.Rdev)) == 0
			if meta.whiteout && (!info.Mode().IsRegular() || info.Size() != 0) {
				return fmt.Errorf("invalid overlay whiteout: %s", relative)
			}
			if meta.opaque && !info.IsDir() {
				return fmt.Errorf("opaque marker on non-directory: %s", relative)
			}
			current = append(current, hiddenEntry{path: relative, size: uint64(info.Size()), layer: layerIndex, mode: info.Mode(), whiteout: meta.whiteout || nativeWhiteout, opaque: meta.opaque})
			if !info.IsDir() {
				return nil
			}
			fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			dir := os.NewFile(uintptr(fd), path)
			defer dir.Close()
			for {
				children, err := dir.ReadDir(128)
				if err != nil && !errors.Is(err, io.EOF) {
					return err
				}
				for _, child := range children {
					if err := visit(filepath.Join(relative, child.Name())); err != nil {
						return err
					}
				}
				if errors.Is(err, io.EOF) {
					return nil
				}
			}
		}
		if err := visit("."); err != nil {
			return nil, fmt.Errorf("scan layer %s: %w", layer.DiffID, err)
		}
		// Apply markers only to older layers, never to files added beside them.
		for _, entry := range current {
			if entry.whiteout || entry.opaque {
				if err := hide(entry.path, entry.opaque); err != nil {
					return nil, err
				}
			}
		}
		for _, entry := range current {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if entry.whiteout || entry.path == "." {
				continue
			}
			old, exists := visible[entry.path]
			if exists && !(entry.mode.IsDir() && old.mode.IsDir()) {
				if err := hide(entry.path, false); err != nil {
					return nil, err
				}
			}
			if _, exists := visible[entry.path]; exists {
				pathBytes -= uint64(len(entry.path))
			}
			if err := reserve(entry.path); err != nil {
				return nil, err
			}
			visible[entry.path] = entry
		}
		for _, entry := range current {
			pathBytes -= uint64(len(entry.path))
		}
	}
	return counts, nil
}
