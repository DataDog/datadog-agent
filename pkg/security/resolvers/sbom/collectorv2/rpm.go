// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package collectorv2 holds sbom related files
package collectorv2

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	rpmdb "github.com/knqyf263/go-rpmdb/pkg"

	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

var rpmdbPaths = []string{
	// Berkeley DB
	"usr/lib/sysimage/rpm/Packages",
	"var/lib/rpm/Packages",

	// NDB
	"usr/lib/sysimage/rpm/Packages.db",
	"var/lib/rpm/Packages.db",

	// SQLite3
	"usr/lib/sysimage/rpm/rpmdb.sqlite",
	"var/lib/rpm/rpmdb.sqlite",
}

// rpmdbWALs lists the write-ahead logs of the SQLite rpm databases, which hold
// the transactions rpm commits until a checkpoint folds them in.
var rpmdbWALs = []string{
	"usr/lib/sysimage/rpm/rpmdb.sqlite-wal",
	"var/lib/rpm/rpmdb.sqlite-wal",
}

// maxRpmdbCopies bounds the copies of an rpm database that changes while it is
// copied.
const maxRpmdbCopies = 3

// testHookRpmdbCopied runs after each copy of an rpm database, for tests.
var testHookRpmdbCopied = func() {}

var errUnexpectedNameFormat = errors.New("unexpected name format")

type rpmScanner struct {
}

func (s *rpmScanner) Name() string {
	return "rpm"
}

func (s *rpmScanner) ListPackages(_ context.Context, root *os.Root) ([]sbomtypes.PackageWithInstalledFiles, error) {
	for _, rpmdbPath := range rpmdbPaths {
		if _, err := root.Stat(rpmdbPath); err != nil {
			continue
		}

		// rpmdb.Open requires a file path, so it reads a copy
		tempDir, tempFilePath, err := copyRpmdb(root, rpmdbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to copy rpmdb: %w", err)
		}
		defer os.RemoveAll(tempDir)

		db, err := rpmdb.Open(tempFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open rpmdb at path %s: %w", rpmdbPath, err)
		}
		defer db.Close()

		pkgs, err := db.ListPackages()
		if err != nil {
			return nil, fmt.Errorf("failed to list packages in rpmdb at path %s: %w", rpmdbPath, err)
		}

		packages := make([]sbomtypes.PackageWithInstalledFiles, 0, len(pkgs))
		for _, pkg := range pkgs {
			files, err := pkg.InstalledFileNames()
			if err != nil {
				return nil, fmt.Errorf("unable to get installed files: %w", err)
			}

			for i, file := range files {
				files[i] = filepath.ToSlash(file)
			}

			var srcVer, srcRel string
			if pkg.SourceRpm != "(none)" && pkg.SourceRpm != "" {
				// source epoch is not included in SOURCERPM
				_, srcVer, srcRel, err = splitFileName(pkg.SourceRpm)
				if err != nil {
					seclog.Warnf("failed to parse source rpm %s: %v", pkg.SourceRpm, err)
				}
			}

			epoch := pkg.EpochNum()

			packages = append(packages, sbomtypes.PackageWithInstalledFiles{
				Package: sbomtypes.Package{
					Name:       pkg.Name,
					Version:    pkg.Version,
					Epoch:      epoch,
					Release:    pkg.Release,
					SrcVersion: srcVer,
					SrcEpoch:   epoch,
					SrcRelease: srcRel,
				},
				InstalledFiles: files,
			})
		}
		return packages, nil
	}

	return nil, fmt.Errorf("no rpmdb found in any of the known paths: %w", os.ErrNotExist)
}

// splitFileName returns a name, version, release, epoch, arch:
//
//	e.g.
//		foo-1.0-1.i386.rpm => foo, 1.0, 1, i386
//		1:bar-9-123a.ia64.rpm => bar, 9, 123a, 1, ia64
//
// https://github.com/rpm-software-management/yum/blob/043e869b08126c1b24e392f809c9f6871344c60d/rpmUtils/miscutils.py#L301
func splitFileName(filename string) (name, ver, rel string, err error) {
	filename = strings.TrimSuffix(filename, ".rpm")

	archIndex := strings.LastIndex(filename, ".")
	if archIndex == -1 {
		return "", "", "", errUnexpectedNameFormat
	}

	relIndex := strings.LastIndex(filename[:archIndex], "-")
	if relIndex == -1 {
		return "", "", "", errUnexpectedNameFormat
	}
	rel = filename[relIndex+1 : archIndex]

	verIndex := strings.LastIndex(filename[:relIndex], "-")
	if verIndex == -1 {
		return "", "", "", errUnexpectedNameFormat
	}
	ver = filename[verIndex+1 : relIndex]

	name = filename[:verIndex]
	return name, ver, rel, nil
}

// copyRpmdb copies the rpm database at path in root into a new directory, and
// returns the directory and the copy. rpmdb.Open opens a SQLite database
// immutable, which skips its write-ahead log, so the copy has the log folded in.
func copyRpmdb(root *os.Root, path string) (dir, db string, err error) {
	dir, err = os.MkdirTemp("", "rpmdb-")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()

	db = filepath.Join(dir, filepath.Base(path))
	wal := path + "-wal"
	for range maxRpmdbCopies {
		before := [2]fileState{statFile(root, path), statFile(root, wal)}
		if err := os.Remove(db + "-wal"); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}
		logged := true
		if err := copyFile(root, wal, db+"-wal"); errors.Is(err, fs.ErrNotExist) {
			logged = false
		} else if err != nil {
			return "", "", err
		}
		if err := copyFile(root, path, db); err != nil {
			return "", "", err
		}
		testHookRpmdbCopied()

		// A checkpoint between the two copies would mix their states.
		if [2]fileState{statFile(root, path), statFile(root, wal)} != before {
			continue
		}
		if logged {
			if err := foldWAL(db, db+"-wal"); err != nil {
				return "", "", fmt.Errorf("failed to fold the write-ahead log of %s: %w", path, err)
			}
		}
		return dir, db, nil
	}
	return "", "", fmt.Errorf("rpm database %s kept changing while copied", path)
}

// fileState is the size and modification time of a file, which a write to the
// file changes, zero for a missing file.
type fileState struct {
	size  int64
	mtime int64
}

func statFile(root *os.Root, path string) fileState {
	info, err := root.Stat(path)
	if err != nil {
		return fileState{}
	}
	return fileState{info.Size(), info.ModTime().UnixNano()}
}

// The SQLite write-ahead log format, https://www.sqlite.org/fileformat2.html#walformat.
const (
	walMagic           = 0x377f0682 // the low bit gives the byte order of the checksums
	walVersion         = 3007000
	walHeaderSize      = 32
	walFrameHeaderSize = 24
)

// walIndex locates the pages of the transactions committed to a SQLite
// write-ahead log.
type walIndex struct {
	pageSize int64
	npage    uint32           // database size in pages after the last commit
	frames   map[uint32]int64 // offset in the log of the last version of each page
}

// readWALIndex reads the SQLite write-ahead log r up to its first invalid frame,
// as SQLite recovers a log. A log without a valid commit gives npage 0.
func readWALIndex(r io.ReaderAt) (walIndex, error) {
	be := binary.BigEndian
	hdr := make([]byte, walHeaderSize)
	if n, err := r.ReadAt(hdr, 0); n < len(hdr) {
		if errors.Is(err, io.EOF) {
			return walIndex{}, nil
		}
		return walIndex{}, err
	}
	magic, pageSize := be.Uint32(hdr), be.Uint32(hdr[8:])
	if magic&^1 != walMagic || be.Uint32(hdr[4:]) != walVersion ||
		pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 {
		return walIndex{}, nil
	}
	order := binary.ByteOrder(binary.LittleEndian)
	if magic&1 != 0 {
		order = be
	}
	s0, s1 := walChecksum(order, 0, 0, hdr[:24])
	if s0 != be.Uint32(hdr[24:]) || s1 != be.Uint32(hdr[28:]) {
		return walIndex{}, nil
	}

	idx := walIndex{pageSize: int64(pageSize), frames: make(map[uint32]int64)}
	pending := make(map[uint32]int64)
	frame := make([]byte, walFrameHeaderSize+pageSize)
	for off := int64(walHeaderSize); ; off += int64(len(frame)) {
		if n, err := r.ReadAt(frame, off); n < len(frame) {
			if errors.Is(err, io.EOF) {
				break
			}
			return walIndex{}, err
		}
		pgno := be.Uint32(frame)
		if pgno == 0 || !bytes.Equal(frame[8:16], hdr[16:24]) {
			break
		}
		s0, s1 = walChecksum(order, s0, s1, frame[:8])
		s0, s1 = walChecksum(order, s0, s1, frame[walFrameHeaderSize:])
		if s0 != be.Uint32(frame[16:]) || s1 != be.Uint32(frame[20:]) {
			break
		}
		pending[pgno] = off + walFrameHeaderSize
		if npage := be.Uint32(frame[4:]); npage != 0 {
			maps.Copy(idx.frames, pending)
			clear(pending)
			idx.npage = npage
		}
	}
	return idx, nil
}

// walChecksum continues the checksum s0, s1 of a SQLite write-ahead log over b,
// read as 32-bit words in order.
func walChecksum(order binary.ByteOrder, s0, s1 uint32, b []byte) (uint32, uint32) {
	for ; len(b) >= 8; b = b[8:] {
		s0 += order.Uint32(b) + s1
		s1 += order.Uint32(b[4:]) + s0
	}
	return s0, s1
}

// foldWAL writes into the SQLite database file at dbPath the pages of the
// transactions committed to its write-ahead log at walPath, as a checkpoint does.
func foldWAL(dbPath, walPath string) error {
	wal, err := os.Open(walPath)
	if err != nil {
		return err
	}
	defer wal.Close()
	idx, err := readWALIndex(wal)
	if err != nil || idx.npage == 0 {
		return err
	}

	db, err := os.OpenFile(dbPath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer db.Close()

	// The seccomp profile the Helm chart gives system-probe denies pwrite64,
	// which WriteAt calls, so the pages go out with Seek and Write.
	page := make([]byte, idx.pageSize)
	for pgno, off := range idx.frames {
		if _, err := wal.ReadAt(page, off); err != nil {
			return err
		}
		if _, err := db.Seek(int64(pgno-1)*idx.pageSize, io.SeekStart); err != nil {
			return err
		}
		if _, err := db.Write(page); err != nil {
			return err
		}
	}
	if err := db.Truncate(int64(idx.npage) * idx.pageSize); err != nil {
		return err
	}
	return db.Close()
}

// copyFile copies the file at src in root to dst.
func copyFile(root *os.Root, src, dst string) (err error) {
	srcFile, err := root.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", src, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy source file %s to %s: %w", src, dst, err)
	}

	// important to handle close errors when writing to a file
	if err = dstFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync %s: %w", dst, err)
	}
	if err = dstFile.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", dst, err)
	}
	return nil
}
