// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package trivy implement a simple overlayfs like filesystem to be able to
// scan through layered filesystems.
package trivy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"syscall"

	"golang.org/x/sys/unix"
)

// whiteoutCharDev is defined as zero and is not const only for testing as it
// is not allowed to mknod a 0/0 char dev in userns.
var whiteoutCharDev uint64 = 0 //nolint:revive

var whiteout *fs.DirEntry

type filesystem struct {
	layers []string
}

type file struct {
	*os.File
	ofs  filesystem
	fi   fs.FileInfo
	name string
}

// NewFS returns a fs.ReadDirFS consisting of merging the given layer paths.
func NewFS(layers []string) interface {
	fs.FS
	fs.ReadDirFS
	fs.StatFS
} {
	return &filesystem{layers[:]}
}

// Open implements fs.StatFS.
func (ofs filesystem) Stat(name string) (fs.FileInfo, error) {
	name = path.Join("/", name)[1:]
	if name == "" {
		name = "."
	}
	_, fi, err := ofs.stat(name)
	return fi, err
}

// Open implements fs.FS.
func (ofs filesystem) Open(name string) (fs.File, error) {
	name = path.Join("/", name)[1:]
	layerIndex, fi, err := ofs.stat(name)
	if err != nil {
		err.(*os.PathError).Op = "open"
		return nil, err
	}
	f, err := os.Open(ofs.path(layerIndex, name))
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	return &file{File: f, ofs: ofs, fi: fi, name: name}, nil
}

func (ofs filesystem) path(layerIndex int, name string) string {
	if !fs.ValidPath(name) {
		panic(fmt.Errorf("unexpected invalid path name %q", name))
	}
	return path.Join(ofs.layers[layerIndex], name)
}

func (ofs filesystem) stat(name string) (int, fs.FileInfo, error) {
	var errf error
	for layerIndex := range ofs.layers {
		fi, err := os.Stat(ofs.path(layerIndex, name))
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTDIR) {
			// When path does not exist, overlayfs does not verify that a
			// whiteout file has been created as one of the parent dir in the
			// current layer. Meaning you can open file from lower dirs even
			// if a whiteout or opaque directory has been created on an upper
			// layer.
			continue
		}
		if err != nil {
			errf = err
			break
		}
		if isWhiteout(fi) {
			break
		}
		return layerIndex, fi, nil
	}
	if errf == nil {
		errf = syscall.ENOENT
	}
	return 0, nil, &os.PathError{Op: "stat", Path: name, Err: errf}
}

// ReadDir implements fs.ReadDirFS.
func (ofs filesystem) ReadDir(name string) ([]fs.DirEntry, error) {
	return ofs.readDirN(name, -1)
}

func (ofs filesystem) readDirN(name string, n int) ([]fs.DirEntry, error) {
	name = path.Join("/", name)[1:]
	if name == "" {
		name = "."
	}

	var entriesMap map[string]*fs.DirEntry
	var err error
	var ok bool
	for layerIndex := range ofs.layers {
		if ok, err = ofs.readDirLayer(layerIndex, name, n, &entriesMap); ok {
			break
		}
	}
	if err == nil && entriesMap == nil {
		err = syscall.ENOENT
	}
	if err != nil {
		return []fs.DirEntry{}, &os.PathError{Op: "readdirent", Path: name, Err: err}
	}

	entries := make([]fs.DirEntry, 0, len(entriesMap))
	for _, entry := range entriesMap {
		if entry != whiteout {
			entries = append(entries, *entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	if n > 0 && len(entries) > n {
		entries = entries[:n]
	}
	return entries, nil
}

func (ofs filesystem) readDirLayer(layerIndex int, name string, n int, entriesMap *map[string]*fs.DirEntry) (bool, error) {
	fullname := ofs.path(layerIndex, name)

	di, err := os.Stat(fullname)
	if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTDIR) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if isWhiteout(di) {
		return true, syscall.ENOENT
	}
	if !di.IsDir() {
		return true, syscall.ENOTDIR
	}

	d, err := os.Open(fullname)
	if err != nil {
		return true, err
	}

	entries, err := d.ReadDir(n)
	if err != nil {
		return true, err
	}
	if *entriesMap == nil {
		*entriesMap = make(map[string]*fs.DirEntry)
	}
	for entryIndex, entry := range entries {
		entryName := entry.Name()
		if _, exists := (*entriesMap)[entryName]; !exists {
			entryPtr := &entries[entryIndex]
			if entry.Type().IsRegular() {
				(*entriesMap)[entryName] = entryPtr
			} else {
				ei, err := entry.Info()
				if err != nil {
					return true, err
				}
				if isWhiteout(ei) {
					(*entriesMap)[entryName] = whiteout
				} else {
					(*entriesMap)[entryName] = entryPtr
				}
			}
		}
	}

	return isOpaqueDir(d), nil
}

// ReadDir implements fs.ReadDirFile.
func (f *file) ReadDir(n int) ([]fs.DirEntry, error) {
	if !f.fi.IsDir() {
		return nil, &os.PathError{Op: "readdirent", Path: f.name, Err: syscall.ENOTDIR}
	}
	return f.ofs.readDirN(f.name, n)
}

// Read implements fs.File.
func (f *file) Read(b []byte) (int, error) {
	return f.File.Read(b)
}

// Stat implements fs.File.
func (f *file) Stat() (fs.FileInfo, error) {
	return f.fi, nil
}

// Close implements fs.File.
func (f *file) Close() error {
	return f.File.Close()
}

var _ fs.ReadDirFile = &file{}

func isWhiteout(fm fs.FileInfo) bool {
	return fm.Mode()&fs.ModeCharDevice != 0 && uint64(fm.Sys().(*syscall.Stat_t).Rdev) == whiteoutCharDev
}

func isOpaqueDir(d *os.File) bool {
	var data [1]byte
	var sz int
	var err error
	for {
		sz, err = unix.Fgetxattr(int(d.Fd()), "trusted.overlay.opaque", data[:])
		if err != unix.EINTR {
			break
		}
	}
	return sz == 1 && data[0] == 'y'
}
