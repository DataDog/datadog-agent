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
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// HiddenBytesLimits bounds work and path storage across an entire image scan.
type HiddenBytesLimits struct {
	MaxEntries      uint64
	MaxPathBytes    uint64
	MaxArchiveBytes uint64
}

// DefaultHiddenBytesLimits returns the maximum supported image scan size.
func DefaultHiddenBytesLimits() HiddenBytesLimits {
	return HiddenBytesLimits{MaxEntries: 500000, MaxPathBytes: 64 << 20, MaxArchiveBytes: 4 << 30}
}

type hiddenFileID struct {
	device uint64
	inode  uint64
}

type hiddenFileLinks struct {
	data  *hiddenData
	links uint64
	seen  uint64
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
	state, err := newHiddenBytesState(ctx, len(layers), limits)
	if err != nil {
		return nil, err
	}
	objects := make(map[hiddenFileID]*hiddenFileLinks)
	for layerIndex, layer := range layers {
		var current []hiddenEntry
		var layerObjects []*hiddenFileLinks
		var visit func(string) error
		visit = func(relative string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := state.trackEntry(relative); err != nil {
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
			entry := hiddenEntry{path: relative, mode: info.Mode(), whiteout: meta.whiteout || nativeWhiteout, opaque: meta.opaque}
			if info.Mode().IsRegular() && !entry.whiteout {
				id := hiddenFileID{device: uint64(stat.Dev), inode: stat.Ino}
				object, exists := objects[id]
				if !exists {
					object = &hiddenFileLinks{data: &hiddenData{size: uint64(info.Size()), layer: layerIndex}, links: uint64(stat.Nlink)}
					objects[id] = object
					layerObjects = append(layerObjects, object)
				}
				if object.data.layer != layerIndex || object.data.size != uint64(info.Size()) || object.links != uint64(stat.Nlink) || stat.Nlink == 0 {
					return errors.New("inconsistent or cross-layer image hardlink metadata")
				}
				object.seen++
				entry.data = object.data
			}
			current = append(current, entry)
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
		for _, object := range layerObjects {
			if object.seen != object.links {
				return nil, errors.New("image hardlink group extends outside its layer")
			}
		}
		if err := state.applyLayer(current); err != nil {
			return nil, err
		}
	}
	return state.counts, nil
}
