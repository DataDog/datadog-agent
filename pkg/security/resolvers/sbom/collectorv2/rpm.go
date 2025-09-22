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
	"os"
	"path/filepath"
	"strings"

	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	rpmdb "github.com/knqyf263/go-rpmdb/pkg"
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

		// sadly, we need to escape the root here :(
		rpmdbFullPath := filepath.Join(root.Name(), rpmdbPath)
		db, err := rpmdb.Open(rpmdbFullPath)
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
