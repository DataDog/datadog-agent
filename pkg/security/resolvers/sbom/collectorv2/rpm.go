// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package collectorv2 holds sbom related files
package collectorv2

import (
	"context"
	"errors"
	"fmt"
	"io"
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

		// copy the rpmdb to a temp file because rpmdb.Open() requires a file path
		tempFilePath, err := writeFileToTemp(root, rpmdbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to write rpmdb to temp file: %w", err)
		}
		defer os.RemoveAll(tempFilePath)

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
			installed, err := pkg.InstalledFiles()
			if err != nil {
				return nil, fmt.Errorf("unable to get installed files: %w", err)
			}
			files := indexedRPMFiles(installed)

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

// unindexedRPMFlags marks the files of a package that its index leaves out: its
// configuration, as dpkg conffiles, the files created at runtime and the docs.
const unindexedRPMFlags = rpmdb.RPMFILE_CONFIG | rpmdb.RPMFILE_GHOST | rpmdb.RPMFILE_DOC | rpmdb.RPMFILE_LICENSE

// indexedRPMFiles returns the paths of the installed files of a package that
// the index of its files holds.
func indexedRPMFiles(installed []rpmdb.FileInfo) []string {
	files := make([]string, 0, len(installed))
	for _, file := range installed {
		if int32(file.Flags)&unindexedRPMFlags != 0 {
			continue
		}
		files = append(files, filepath.ToSlash(file.Path))
	}
	return files
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

func writeFileToTemp(root *os.Root, path string) (_ string, err error) {
	srcFile, err := root.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open source file %s: %w", path, err)
	}
	defer srcFile.Close()

	tmpFile, err := os.CreateTemp("", "rpmdb-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err != nil {
			// remove any partially-written file
			os.Remove(tmpFile.Name())
		}
	}()
	defer tmpFile.Close()

	if _, err = io.Copy(tmpFile, srcFile); err != nil {
		return "", fmt.Errorf("failed to copy source file %s to temp file: %w", path, err)
	}

	// important to handle close errors when writing to a file
	if err = tmpFile.Sync(); err != nil {
		return "", fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err = tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	return tmpFile.Name(), nil
}
